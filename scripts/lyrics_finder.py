#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
lyrics_finder.py —— 通过百度搜索结果抓取歌曲歌词（纯标准库，无第三方依赖）

随本项目独立分发（纯标准库）；后端 backend/handlers/lyrics_fetch.go 会调用它。
相对上游脚本的改动仅有两处（均向后兼容）：
  1) 批量模式支持 --json，输出 {"items": [...]} 结构，便于后端解析
  2) 新增 --delay，控制批量模式下每首之间的间隔（默认 1.0 秒）

链路：
  1) 百度搜索「<歌名> 歌词」，解析页面内嵌的音乐卡片数据（最快、最准）
  2) 没有卡片时，走网易云公开接口（搜索 + 歌词）兜底
  3) 再不行才抓百度自然结果里的歌词页，按歌词特征打分抽取正文

用法示例：
  python3 lyrics_finder.py 倔强 -a 五月天
  python3 lyrics_finder.py "晴天" --artist 周杰伦 --json
  python3 lyrics_finder.py 起风了 -e netease --raw-lrc
  python3 lyrics_finder.py --file songs.txt -d ./lyrics
"""

from __future__ import annotations

import argparse
import difflib
import gzip
import html as htmllib
import json
import os
import re
import sys
import time
import unicodedata
import urllib.error
import urllib.parse
import urllib.request
import zlib
from dataclasses import dataclass, field
from html.parser import HTMLParser
from http.cookiejar import CookieJar
from typing import Iterable

UA = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)
BAIDU_HOME = "https://www.baidu.com/"
BAIDU_SEARCH = "https://www.baidu.com/s"
NETEASE_SEARCH = "https://music.163.com/api/search/get/web"
NETEASE_LYRIC = "https://music.163.com/api/song/lyric"
QQ_SEARCH = "https://c.y.qq.com/soso/fcgi-bin/client_search_cp"
QQ_LYRIC = "https://c.y.qq.com/lyric/fcgi-bin/fcg_query_lyric_new.fcg"


# --------------------------------------------------------------------------- #
# HTTP
# --------------------------------------------------------------------------- #
class HttpClient:
    """带 Cookie 的极简 HTTP 客户端，处理 gzip/deflate 与编码探测。"""

    def __init__(self, timeout: float = 15.0, debug: bool = False) -> None:
        self.timeout = timeout
        self.debug = debug
        self.jar = CookieJar()
        self.opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(self.jar)
        )
        self._baidu_warmed = False

    def get(self, url: str, referer: str | None = None, retries: int = 2) -> tuple[str, str]:
        last_exc: Exception | None = None
        for attempt in range(retries + 1):
            try:
                return self._get_once(url, referer)
            except urllib.error.HTTPError as exc:
                last_exc = exc
                if exc.code not in (408, 429, 500, 502, 503, 504):
                    raise
            except (urllib.error.URLError, TimeoutError, ConnectionError) as exc:
                last_exc = exc
            if attempt < retries:
                time.sleep(1.0 * (attempt + 1))
        raise last_exc if last_exc else RuntimeError(f"请求失败: {url}")

    def _get_once(self, url: str, referer: str | None = None) -> tuple[str, str]:
        headers = {
            "User-Agent": UA,
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
            "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
            "Accept-Encoding": "gzip, deflate",
            "Connection": "close",
        }
        if referer:
            headers["Referer"] = referer
        req = urllib.request.Request(url, headers=headers)
        with self.opener.open(req, timeout=self.timeout) as resp:
            raw = resp.read()
            encoding = (resp.headers.get("Content-Encoding") or "").lower()
            final_url = resp.geturl()
            charset = resp.headers.get_content_charset()

        if "gzip" in encoding:
            raw = gzip.decompress(raw)
        elif "deflate" in encoding:
            try:
                raw = zlib.decompress(raw)
            except zlib.error:
                raw = zlib.decompress(raw, -zlib.MAX_WBITS)

        text = self._decode(raw, charset)
        if self.debug:
            print(f"[http] {final_url} <- {url} ({len(text)} chars)", file=sys.stderr)
        return final_url, text

    @staticmethod
    def _decode(raw: bytes, charset: str | None) -> str:
        candidates: list[str] = []
        if charset:
            candidates.append(charset)
        head = raw[:4096].decode("ascii", "ignore")
        m = re.search(r'charset=["\']?([\w-]+)', head, re.I)
        if m:
            candidates.append(m.group(1))
        candidates += ["utf-8", "gb18030"]

        for enc in candidates:
            enc = enc.lower()
            if enc in ("gb2312", "gbk"):
                enc = "gb18030"
            try:
                return raw.decode(enc)
            except (UnicodeDecodeError, LookupError):
                continue
        return raw.decode("utf-8", "ignore")

    def warm_baidu(self) -> None:
        """先访问首页拿 BAIDUID，降低被判定为爬虫的概率。"""
        if self._baidu_warmed:
            return
        try:
            self.get(BAIDU_HOME)
        except Exception:
            pass
        self._baidu_warmed = True


# --------------------------------------------------------------------------- #
# 百度搜索
# --------------------------------------------------------------------------- #
def build_baidu_url(query: str, pn: int = 0) -> str:
    """参数与浏览器里搜出来的地址保持一致。"""
    params = {
        "ie": "utf-8",
        "f": "8",
        "rsv_bp": "1",
        "rsv_idx": "1",
        "tn": "baidu",
        "wd": query,
        "rn": "10",
    }
    if pn:
        params["pn"] = str(pn)
    return f"{BAIDU_SEARCH}?{urllib.parse.urlencode(params)}"


def iter_sdata(html: str) -> Iterable[dict]:
    """遍历页面里 <!--s-data:{...}--> 内嵌的 JSON。"""
    decoder = json.JSONDecoder()
    marker = "<!--s-data:"
    idx = html.find(marker)
    while idx >= 0:
        start = idx + len(marker)
        chunk = html[start : start + 2_000_000]
        try:
            obj, _ = decoder.raw_decode(chunk.lstrip())
        except ValueError:
            obj = None
        if isinstance(obj, dict):
            yield obj
        idx = html.find(marker, start)


def find_lrc_cards(html: str) -> list[dict]:
    """从内嵌数据里捞出带歌词的音乐卡片。"""
    cards: list[dict] = []
    for obj in iter_sdata(html):
        for node in walk_dicts(obj):
            lrc = node.get("lrc")
            if not isinstance(lrc, dict):
                continue
            lines = lrc.get("lrcArr") or lrc.get("lrcContent")
            if isinstance(lines, str):
                lines = [ln for ln in lines.splitlines() if ln.strip()]
            if not isinstance(lines, list) or len(lines) < 3:
                continue
            song = node.get("song") if isinstance(node.get("song"), dict) else {}
            album = song.get("album")
            if isinstance(album, dict):
                album = album.get("albumName") or album.get("name") or ""
            cards.append(
                {
                    "song": str(song.get("songName") or song.get("name") or "").strip(),
                    "singer": str(
                        song.get("allSingerName")
                        or song.get("singer")
                        or song.get("artistName")
                        or ""
                    ).strip(),
                    "album": str(album or "").strip(),
                    "site": str(song.get("siteName") or "").strip(),
                    "url": str(song.get("songLink") or song.get("lrcUrl") or "").strip(),
                    "lines": [str(x).strip() for x in lines if str(x).strip()],
                }
            )
    return cards


def walk_dicts(node: object, depth: int = 0) -> Iterable[dict]:
    if depth > 6:
        return
    if isinstance(node, dict):
        yield node
        for value in node.values():
            yield from walk_dicts(value, depth + 1)
    elif isinstance(node, list):
        for item in node[:80]:
            yield from walk_dicts(item, depth + 1)


TAG_RE = re.compile(r"<[^>]+>")


def strip_tags(fragment: str) -> str:
    return htmllib.unescape(TAG_RE.sub("", fragment)).strip()


def parse_organic_results(html: str) -> list[dict]:
    """抓取自然结果的标题 / 链接 / 摘要。"""
    results: list[dict] = []
    seen: set[str] = set()
    for m in re.finditer(r"<h3[^>]*>([\s\S]{0,600}?)</h3>", html):
        block = m.group(1)
        a = re.search(r'<a[^>]*href="([^"]+)"', block)
        if not a:
            continue
        url = htmllib.unescape(a.group(1))
        title = strip_tags(block)
        if not title or url in seen:
            continue
        if url.startswith("/"):  # 站内聚合页，跳过
            continue
        seen.add(url)

        # 摘要：h3 之后的一段文字
        tail = html[m.end() : m.end() + 4000]
        abstract = ""
        sm = re.search(
            r'class="[^"]*(?:c-abstract|content-right_|c-span-last)[^"]*"[^>]*>([\s\S]{0,1200}?)</div>',
            tail,
        )
        if sm:
            abstract = strip_tags(sm.group(1))
        results.append({"title": title, "url": url, "abstract": abstract[:300]})
    return results


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):  # noqa: D401
        return None


def resolve_target(client: HttpClient, url: str) -> str:
    """把百度 /link?url=... 跳转解析成真实地址。"""
    if "baidu.com/link?" not in url:
        return url
    opener = urllib.request.build_opener(_NoRedirect)
    req = urllib.request.Request(
        url, headers={"User-Agent": UA, "Referer": BAIDU_HOME}
    )
    resolved = ""
    try:
        with opener.open(req, timeout=client.timeout) as resp:
            resolved = resp.headers.get("Location") or ""
            body = resp.read(20000).decode("utf-8", "ignore") if not resolved else ""
    except urllib.error.HTTPError as exc:
        resolved = (exc.headers.get("Location") if exc.headers else "") or ""
        body = ""
    except Exception:
        body = ""

    if not resolved:
        m = re.search(
            r"""(?:window\.location\.replace|location\.href\s*=)\s*\(?["']([^"']+)""",
            body,
        )
        resolved = m.group(1) if m else ""
    if not resolved:
        return url
    resolved = htmllib.unescape(resolved)
    # HTTP 头按 latin-1 解码，含中文的 Location 会变乱码，先尝试还原
    try:
        fixed = resolved.encode("latin-1").decode("utf-8")
        if any("\u4e00" <= ch <= "\u9fff" for ch in fixed):
            resolved = fixed
    except (UnicodeEncodeError, UnicodeDecodeError):
        pass
    # 再对仍未编码的字符做一次转义，urllib 遇到非 ascii 会直接抛错
    return urllib.parse.quote(resolved, safe=":/?#[]@!$&'()*+,;=%~-._")


# --------------------------------------------------------------------------- #
# 页面正文里的歌词抽取
# --------------------------------------------------------------------------- #
HINT_RE = re.compile(
    r"(lrc|lyric|geci|gc-|zrcil|song-?text|poem|para|article-content|content-?text)",
    re.I,
)
VOID_TAGS = {
    "area", "base", "br", "col", "embed", "hr", "img", "input",
    "link", "meta", "param", "source", "track", "wbr",
}
JUNK_LINE = re.compile(
    r"(下载|点击|更多|扫一扫|二维码|广告|上一篇|下一篇|分享到|关注|版权|"
    r"http|www\.|\.com|浏览|阅读|编辑于|举报|评论|纯音乐|"
    r"\{if|\$\{|class=|</|function\s*\(|var\s|window\.)",
    re.I,
)
TS_RE = re.compile(r"\[\d{1,2}:\d{2}(?:[.:]\d{1,3})?\]")
META_RE = re.compile(r"\[(?:ti|ar|al|by|offset|re|ve|kana|length):[^\]]*\]", re.I)
CN_RE = re.compile(r"[\u4e00-\u9fff]")


class BlockParser(HTMLParser):
    """把 class/id 命中歌词特征的容器整段文本收集出来。"""

    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.depth = 0
        self.skip = 0
        self.captures: list[dict] = []
        self.blocks: list[str] = []

    def handle_starttag(self, tag, attrs):
        if tag in ("script", "style", "noscript"):
            self.skip += 1
            return
        if self.skip:
            return
        if tag == "br":
            self.handle_data("\n")
            return
        if tag in VOID_TAGS:
            return
        self.depth += 1
        signature = " ".join(f"{k}={v}" for k, v in attrs if v)
        if HINT_RE.search(signature):
            self.captures.append({"depth": self.depth, "buf": []})

    def handle_startendtag(self, tag, attrs):
        if tag == "br":
            self.handle_data("\n")

    def handle_endtag(self, tag):
        if tag in ("script", "style", "noscript"):
            self.skip = max(0, self.skip - 1)
            return
        if self.skip or tag in VOID_TAGS:
            return
        for cap in list(self.captures):
            if cap["depth"] == self.depth:
                self.blocks.append("".join(cap["buf"]))
                self.captures.remove(cap)
        self.depth = max(0, self.depth - 1)

    def handle_data(self, data):
        if self.skip:
            return
        for cap in self.captures:
            cap["buf"].append(data)


def clean_lines(text: str, keep_timestamps: bool = False) -> list[str]:
    out: list[str] = []
    for raw in text.replace("\r", "\n").split("\n"):
        line = raw.replace("\u3000", " ").strip()
        if not keep_timestamps:
            line = TS_RE.sub("", line)
            line = META_RE.sub("", line).strip()
        line = re.sub(r"\s{2,}", " ", line)
        if not line or len(line) > 120 or JUNK_LINE.search(line):
            continue
        out.append(line)
    # 去掉连续重复行
    dedup: list[str] = []
    for line in out:
        if not dedup or dedup[-1] != line:
            dedup.append(line)
    return dedup


def score_lines(lines: list[str], song: str = "", singer: str = "") -> float:
    if len(lines) < 4:
        return 0.0
    joined = "\n".join(lines)
    cn = len(CN_RE.findall(joined))
    ratio = cn / max(1, len(joined))
    avg_len = sum(len(x) for x in lines) / len(lines)
    score = 0.0
    score += min(len(lines), 25) * 0.6            # 行数：歌词通常 10~60 行
    score += ratio * 6                            # 中文占比
    score += 3 if 6 <= avg_len <= 26 else 0        # 行长度像歌词
    score += 3 if re.search(r"(作词|作曲|编曲|词[:：]|曲[:：])", joined) else 0
    score -= 4 if len(lines) < 6 else 0
    score -= 3 if avg_len > 45 else 0
    if song and song in joined:
        score += 1.5
    if singer and singer in joined:
        score += 1.5
    if len(set(lines)) < len(lines) * 0.5:
        score += 1.0                              # 副歌重复是歌词特征

    # 导航条/菜单项的典型特征是大量 2~4 字的短词
    short_ratio = sum(1 for ln in lines if len(ln) <= 4) / len(lines)
    score -= short_ratio * 14

    # 百科/文章正文会混进来：句子成段（句号多）、带目录编号，这些都要压分
    periods = joined.count("。")
    if periods >= 3:
        score -= min(periods, 10) * 1.5
    outline = sum(1 for ln in lines if re.match(r"^\d+[\u4e00-\u9fff]", ln))
    if outline >= 2:
        score -= min(outline, 8) * 2.0
    if re.search(r"(百科|目录|参考资料|简介|早年|演艺经历|发行时间)", joined):
        score -= 4
    return score


def extract_lyrics_from_html(
    html: str, song: str = "", singer: str = "", keep_timestamps: bool = False
) -> tuple[list[str], float]:
    parser = BlockParser()
    try:
        parser.feed(html)
        parser.close()
    except Exception:
        pass

    candidates: list[str] = list(parser.blocks)

    # 兜底：整页纯文本。先把块级标签换成换行，否则整页会被压成一行
    body = re.sub(r"<(script|style)[\s\S]*?</\1>", " ", html, flags=re.I)
    body = re.sub(r"(?i)<(?:br|hr)\s*/?>|</(?:p|div|li|h[1-6]|tr|td|dd|section)>", "\n", body)
    text_all = strip_tags(body)
    candidates.append(text_all)

    best_lines: list[str] = []
    best_score = 0.0
    block_count = len(parser.blocks)
    for idx, chunk in enumerate(candidates):
        lines = clean_lines(chunk, keep_timestamps)
        s = score_lines(lines, song, singer)
        if idx < block_count and s > 0:
            s += 4  # 命中 class/id 像歌词容器的，优先
        if s > best_score:
            best_score, best_lines = s, lines
    return best_lines, best_score


# --------------------------------------------------------------------------- #
# 网易云兜底
# --------------------------------------------------------------------------- #
def norm(text: str) -> str:
    text = unicodedata.normalize("NFKC", text or "").lower()
    return re.sub(r"[\s\-_·、,，.。'\"“”‘’()（）\[\]【】]", "", text)


def title_match(query: str, title: str) -> int:
    """歌名匹配度：2=同名/包含，1=高度相似，0=不像同一首。"""
    a, b = norm(query), norm(title)
    if not a or not b:
        return 1
    if a == b or a in b or b in a:
        return 2
    if abs(len(a) - len(b)) > 2:
        return 0
    if difflib.SequenceMatcher(None, a, b).ratio() >= 0.85:
        return 1
    return 0


def _json_get(client: HttpClient, url: str, referer: str = "https://music.163.com/") -> dict | None:
    """接口偶尔会返回 HTML/空内容，重试一次再放弃。"""
    for attempt in range(2):
        try:
            _, body = client.get(url, referer=referer)
            if body.lstrip().startswith("{"):
                return json.loads(body)
        except Exception:
            pass
        if attempt == 0:
            time.sleep(1.0)
    return None


def netease_lyrics(
    client: HttpClient, song: str, singer: str = "", keep_timestamps: bool = False
) -> dict | None:
    query = f"{song} {singer}".strip()
    data = None
    for endpoint in (NETEASE_SEARCH, "https://music.163.com/api/search/get"):
        data = _json_get(
            client,
            f"{endpoint}?{urllib.parse.urlencode({'s': query, 'type': 1, 'limit': 10})}",
        )
        if data and (data.get("result") or {}).get("songs"):
            break
    if not data:
        return None

    songs = (data.get("result") or {}).get("songs") or []
    if not songs:
        return None

    def rank(item: dict) -> tuple:
        name = item.get("name", "")
        artists = norm("/".join(a.get("name", "") for a in item.get("artists", [])))
        score = title_match(song, name) * 3
        if norm(name) == norm(song):
            score += 3                      # 完全同名优先
        if re.search(r"(伴奏|纯音乐|remix|inst\.?|karaoke|cover|版|片段|DJ|女声|男声)", name, re.I):
            score -= 3                      # 各种改编/翻唱降权
        if singer and norm(singer) in artists:
            score += 4
        return (score, item.get("popularity", 0) or 0)

    # 搜索结果里常有翻唱/伴奏/纯音乐，按匹配度排序后逐个验证歌词
    for best in sorted(songs, key=rank, reverse=True)[:6]:
        name = best.get("name", "")
        if title_match(song, name) == 0:
            continue
        if re.search(r"(伴奏|纯音乐|remix|inst\.?|karaoke|cover)", name, re.I):
            continue
        artists = "/".join(a.get("name", "") for a in best.get("artists", []))
        # 明确指定了歌手时，不接受别人的翻唱（网易云缺版权时会返回一堆翻唱）
        if singer and singer not in artists and norm(singer) not in norm(artists):
            continue
        lyric_url = f"{NETEASE_LYRIC}?{urllib.parse.urlencode({'id': best['id'], 'lv': 1, 'kv': 1, 'tv': -1})}"
        try:
            _, lbody = client.get(lyric_url, referer="https://music.163.com/")
            ldata = json.loads(lbody)
        except Exception:
            continue

        raw = (ldata.get("lrc") or {}).get("lyric") or ""
        if not raw.strip():
            continue

        if keep_timestamps:
            lines = [ln.strip() for ln in raw.splitlines() if ln.strip()]
        else:
            lines = clean_lines(raw)
        if len(lines) < 5:
            continue

        return {
            "song": name or song,
            "singer": "/".join(a.get("name", "") for a in best.get("artists", [])),
            "album": (best.get("album") or {}).get("name", ""),
            "source": "网易云音乐",
            "url": f"https://music.163.com/song?id={best['id']}",
            "engine": "netease",
            "lines": lines,
        }
    return None


def netease_lyric_by_id(
    client: HttpClient, song_id: str, keep_timestamps: bool = False
) -> list[str]:
    """结果页命中网易云歌曲页时，直接调歌词接口拿纯歌词。"""
    url = f"{NETEASE_LYRIC}?{urllib.parse.urlencode({'id': song_id, 'lv': 1, 'kv': 1, 'tv': -1})}"
    _, body = client.get(url, referer="https://music.163.com/")
    data = json.loads(body)
    raw = (data.get("lrc") or {}).get("lyric") or ""
    if not raw.strip():
        return []
    if keep_timestamps:
        return [ln.strip() for ln in raw.splitlines() if ln.strip()]
    lines = clean_lines(raw)
    return lines if len(lines) >= 5 else []


def qq_lyrics(
    client: HttpClient, song: str, singer: str = "", keep_timestamps: bool = False
) -> dict | None:
    """QQ 音乐：补充网易云没有版权的歌（如周杰伦）。"""
    query = f"{song} {singer}".strip()
    data = _json_get(
        client,
        f"{QQ_SEARCH}?{urllib.parse.urlencode({'p': 1, 'n': 12, 'w': query, 'format': 'json'})}",
        referer="https://y.qq.com/",
    )
    if not data:
        return None

    songs = (((data.get("data") or {}).get("song") or {}).get("list")) or []
    if not songs:
        return None

    def rank(item: dict) -> tuple:
        name = item.get("songname", "")
        artists = "/".join(a.get("name", "") for a in item.get("singer", []))
        score = title_match(song, name) * 3
        if norm(name) == norm(song):
            score += 3
        if re.search(r"(伴奏|纯音乐|remix|inst\.?|karaoke|cover|版|片段|dj|女声|男声)", name, re.I):
            score -= 3
        if singer and norm(singer) in norm(artists):
            score += 4
        return (score, 0)

    for item in sorted(songs, key=rank, reverse=True)[:6]:
        name = item.get("songname", "")
        artists = "/".join(a.get("name", "") for a in item.get("singer", []))
        if title_match(song, name) == 0:
            continue
        if re.search(r"(伴奏|纯音乐|remix|inst\.?|karaoke|cover)", name, re.I):
            continue
        if singer and norm(singer) not in norm(artists):
            continue

        mid = item.get("songmid", "")
        lyric = _json_get(
            client,
            f"{QQ_LYRIC}?{urllib.parse.urlencode({'songmid': mid, 'format': 'json', 'nobase64': 1, 'g_tk': 5381})}",
            referer="https://y.qq.com/portal/player.html",
        )
        raw = (lyric or {}).get("lyric") or ""
        if not raw.strip():
            continue
        if keep_timestamps:
            lines = [ln.strip() for ln in raw.splitlines() if ln.strip()]
        else:
            lines = clean_lines(raw)
        if len(lines) < 5:
            continue

        return {
            "song": name or song,
            "singer": artists,
            "album": item.get("albumname", ""),
            "source": "QQ音乐",
            "url": f"https://y.qq.com/n/ryqq/songDetail/{mid}",
            "engine": "qq",
            "lines": lines,
        }
    return None


# --------------------------------------------------------------------------- #
# 主流程
# --------------------------------------------------------------------------- #
@dataclass
class Result:
    song: str
    singer: str
    lines: list[str]
    engine: str
    source: str = ""
    url: str = ""
    album: str = ""
    meta: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        return {
            "song": self.song,
            "singer": self.singer,
            "album": self.album,
            "engine": self.engine,
            "source": self.source,
            "url": self.url,
            "line_count": len(self.lines),
            "lyrics": "\n".join(self.lines),
        }


def build_query(song: str, singer: str = "") -> str:
    q = f"{song} {singer}".strip() if singer else song
    if "歌词" not in q:
        q += " 歌词"
    return q


class BaiduBlocked(RuntimeError):
    """百度返回验证页/空页时抛出。"""


def is_baidu_blocked(html: str) -> bool:
    if not html or len(html) < 8000:
        return True
    if any(k in html for k in ("百度安全验证", "wappass.baidu.com", "请完成下方验证", "网络不给力")):
        return True
    return "<!--s-data:" not in html and 'class="result' not in html and "<h3" not in html


def find_lyrics(
    client: HttpClient,
    song: str,
    singer: str = "",
    engine: str = "auto",
    keep_timestamps: bool = False,
    page: int = 0,
    debug: bool = False,
    query_override: str | None = None,
) -> Result | None:
    """auto: 百度音乐卡片 -> 网易云 -> QQ音乐 -> 百度自然结果页。"""
    baidu_html = ""
    baidu_url = ""

    if engine not in ("netease", "qq"):
        client.warm_baidu()
        query = query_override or build_query(song, singer)
        baidu_url = build_baidu_url(query, pn=page * 10)
        if debug:
            print(f"[baidu] {baidu_url}", file=sys.stderr)
        _, baidu_html = client.get(baidu_url, referer=BAIDU_HOME)
        if is_baidu_blocked(baidu_html):
            if engine == "auto":
                if debug:
                    print("[baidu] 返回验证页/空页，跳过百度链路", file=sys.stderr)
                baidu_html = ""
            else:
                raise BaiduBlocked(
                    "百度返回了验证页或空页面（可能触发了风控）。"
                    "等几分钟再试，或改用 -e netease / -e qq。"
                )

    # 阶段 1：百度页内嵌音乐卡片（最准）
    if engine in ("auto", "baidu", "card") and baidu_html:
        best: Result | None = None
        for card in find_lrc_cards(baidu_html):
            lines = clean_lines("\n".join(card["lines"]), keep_timestamps)
            if len(lines) < 4:
                continue
            # 卡片未必是同一首歌（搜「倔强 云朵」却给五月天），核对一次歌名
            if card["song"] and title_match(song, card["song"]) == 0:
                continue
            score = score_lines(lines, song, singer)
            if singer and card["singer"] and norm(singer) not in norm(card["singer"]):
                score -= 2
            if debug:
                print(f"[card] {score:5.1f} {len(lines):3d} 行  {card['song']}", file=sys.stderr)
            candidate = Result(
                song=card["song"] or song,
                singer=card["singer"] or singer,
                lines=lines,
                engine="baidu-card",
                source=card["site"] or "百度音乐卡片",
                url=card["url"],
                album=card["album"],
                meta={"score": round(score, 2)},
            )
            if best is None or score > best.meta.get("score", 0):
                best = candidate
        if best:
            return best

    # 阶段 2：网易云公开接口（稳定、无需反爬处理）
    if engine in ("auto", "netease"):
        try:
            data = netease_lyrics(client, song, singer, keep_timestamps)
        except Exception as exc:
            if debug:
                print(f"[netease] {exc}", file=sys.stderr)
            data = None
        if data:
            return Result(
                song=data["song"],
                singer=data["singer"],
                lines=data["lines"],
                engine="netease",
                source=data["source"],
                url=data["url"],
                album=data["album"],
            )
        if debug:
            print("[netease] 未命中", file=sys.stderr)

    # 阶段 3：QQ 音乐（补网易云缺失的版权歌）
    if engine in ("auto", "qq"):
        try:
            data = qq_lyrics(client, song, singer, keep_timestamps)
        except Exception as exc:
            if debug:
                print(f"[qq] {exc}", file=sys.stderr)
            data = None
        if data:
            return Result(
                song=data["song"],
                singer=data["singer"],
                lines=data["lines"],
                engine="qq",
                source=data["source"],
                url=data["url"],
                album=data["album"],
            )
        if debug:
            print("[qq] 未命中", file=sys.stderr)

    # 阶段 4：百度自然结果里的歌词站，按歌词特征打分挑最好的一段

    # 阶段 4：百度自然结果里的歌词站，按歌词特征打分挑最好的一段
    if engine in ("auto", "baidu", "page") and baidu_html:
        page_result: Result | None = None
        skip_hosts = ("wenku.baidu.com", "y.qq.com")
        for item in parse_organic_results(baidu_html)[:6]:
            try:
                real = resolve_target(client, item["url"])
            except Exception as exc:
                if debug:
                    print(f"[skip] {item['url']} {exc}", file=sys.stderr)
                continue
            host = urllib.parse.urlsplit(real).netloc
            if any(host.endswith(h) for h in skip_hosts):
                continue

            # 网易云歌曲页：HTML 是 JS 渲染的，改走接口
            if host.endswith("music.163.com"):
                sid = (urllib.parse.parse_qs(urllib.parse.urlsplit(real).query).get("id") or [""])[0]
                if not sid:
                    continue
                try:
                    lines = netease_lyric_by_id(client, sid, keep_timestamps)
                except Exception:
                    lines = []
                score = score_lines(lines, song, singer) if lines else 0
                if debug:
                    print(f"[163] {score:5.1f} {len(lines):3d} 行  {real}", file=sys.stderr)
            else:
                try:
                    _, sub = client.get(real, referer=baidu_url)
                except Exception as exc:
                    if debug:
                        print(f"[skip] {item['url']} {exc}", file=sys.stderr)
                    continue
                lines, score = extract_lyrics_from_html(sub, song, singer, keep_timestamps)
                if debug:
                    print(f"[page] {score:5.1f} {len(lines):3d} 行  {real}", file=sys.stderr)

            if score >= 14 and (
                page_result is None or score > page_result.meta.get("score", 0)
            ):
                page_result = Result(
                    song=item["title"].split("-")[0].strip() or song,
                    singer=singer,
                    lines=lines,
                    engine="baidu-page",
                    source=host,
                    url=real,
                    meta={"score": round(score, 2)},
                )
            time.sleep(0.6)
        if page_result:
            return page_result
    return None


def render(result: Result) -> str:
    who = f" - {result.singer}" if result.singer else ""
    head = f"{result.song}{who}"
    album = f"《{result.album}》" if result.album else ""
    source = result.source or "-"
    bar = "-" * min(60, max(24, len(head) * 2))
    lines = [head + (f"  {album}" if album else ""), f"来源：{source}  [{result.engine}]"]
    if result.url:
        lines.append(f"链接：{result.url}")
    lines += [bar, "\n".join(result.lines), bar, f"共 {len(result.lines)} 行"]
    return "\n".join(lines)


def safe_filename(name: str) -> str:
    name = re.sub(r'[\\/:*?"<>|\s]+', "_", name).strip("_")
    return name[:80] or "lyrics"


def parse_song_line(line: str) -> tuple[str, str]:
    for sep in (" - ", " – ", "|", "——", "\t"):
        if sep in line:
            left, right = line.split(sep, 1)
            return left.strip(), right.strip()
    parts = line.split()
    if len(parts) >= 2:
        return " ".join(parts[:-1]), parts[-1]
    return line.strip(), ""


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(
        description="通过百度搜索抓取歌曲歌词（无第三方依赖）",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=(
            "示例：\n"
            "  %(prog)s 倔强 -a 五月天\n"
            "  %(prog)s 晴天 --artist 周杰伦 --json\n"
            "  %(prog)s 起风了 -e netease --raw-lrc\n"
            "  %(prog)s --file songs.txt -d ./lyrics\n"
        ),
    )
    ap.add_argument("song", nargs="*", help="歌名（可加歌手）")
    ap.add_argument("-a", "--artist", default="", help="歌手名，用于提高命中率")
    ap.add_argument(
        "-e",
        "--engine",
        default="auto",
        choices=["auto", "baidu", "card", "page", "netease", "qq"],
        help="auto=百度卡片>网易云>QQ音乐>百度结果页；card/page 只用百度；netease、qq 只用对应接口",
    )
    ap.add_argument("--json", action="store_true", help="输出 JSON")
    ap.add_argument("-o", "--output", help="写入文件")
    ap.add_argument("-d", "--outdir", help="配合 --file，批量保存目录")
    ap.add_argument("--file", help="批量模式：每行一首，支持「歌名 - 歌手」")
    ap.add_argument("--raw-lrc", action="store_true", help="保留 [00:12.34] 时间轴")
    ap.add_argument("-p", "--page", type=int, default=0, help="百度结果页码，从 0 开始")
    ap.add_argument("--timeout", type=float, default=15.0, help="单请求超时秒数")
    ap.add_argument("--delay", type=float, default=1.0, help="批量模式下每首之间的间隔秒数")
    ap.add_argument("--debug", action="store_true", help="打印抓取过程")
    args = ap.parse_args(argv)

    client = HttpClient(timeout=args.timeout, debug=args.debug)

    # 批量模式
    if args.file:
        if not os.path.exists(args.file):
            print(f"找不到文件：{args.file}", file=sys.stderr)
            return 2
        outdir = args.outdir or "lyrics"
        if not args.json:
            os.makedirs(outdir, exist_ok=True)
        ok = fail = 0
        items: list[dict] = []
        with open(args.file, encoding="utf-8") as fh:
            lines = [ln.strip() for ln in fh if ln.strip() and not ln.startswith("#")]
        for raw in lines:
            song, singer = parse_song_line(raw)
            try:
                result = find_lyrics(
                    client, song, singer, args.engine, args.raw_lrc, args.page, args.debug
                )
            except Exception as exc:
                print(f"[x] {raw}: {exc}", file=sys.stderr)
                result = None
            if not result:
                # --json 模式下 stdout 只留 JSON，进度一律走 stderr
                print(f"[x] {raw}: 未找到", file=sys.stderr if args.json else sys.stdout)
                fail += 1
                if args.json:
                    items.append({"query": raw, "song": song, "artist": singer, "error": "未找到"})
                continue
            if args.json:
                items.append({"query": raw, **result.to_dict()})
                print(f"[v] {raw} ({len(result.lines)} 行)", file=sys.stderr)
            else:
                name = safe_filename(
                    f"{result.song}_{result.singer}" if result.singer else result.song
                )
                path = os.path.join(outdir, name + ".txt")
                with open(path, "w", encoding="utf-8") as fh:
                    fh.write(render(result) + "\n")
                print(f"[v] {raw} -> {path} ({len(result.lines)} 行)")
            ok += 1
            time.sleep(max(0.0, args.delay))
        if args.json:
            print(
                json.dumps(
                    {"ok": ok, "failed": fail, "items": items}, ensure_ascii=False
                )
            )
        else:
            print(f"完成：成功 {ok} 首，失败 {fail} 首，目录 {outdir}")
        return 0 if ok else 1

    song = " ".join(args.song).strip()
    if not song:
        ap.print_help()
        return 2

    # 先「歌名+歌手+歌词」搜；没结果再只换搜索词（仍锁定歌手），百度卡片对 query 敏感
    attempts: list[str | None] = [None]
    if args.artist and args.engine == "auto":
        attempts.append(f"{song} 歌词")

    result = None
    for idx, query_override in enumerate(attempts):
        if idx:
            time.sleep(1.5)
        try:
            result = find_lyrics(
                client,
                song,
                args.artist,
                args.engine,
                args.raw_lrc,
                args.page,
                args.debug,
                query_override=query_override,
            )
        except urllib.error.HTTPError as exc:
            print(f"请求失败：HTTP {exc.code}（可能被限流，稍后再试或换 -e netease）", file=sys.stderr)
            return 1
        except urllib.error.URLError as exc:
            print(f"网络错误：{exc.reason}", file=sys.stderr)
            return 1
        except BaiduBlocked as exc:
            print(str(exc), file=sys.stderr)
            return 1
        if result:
            break

    if not result:
        print("没有找到歌词。可以试试：加上 -a 歌手名、换 -e netease、或用 -p 1 翻页。", file=sys.stderr)
        return 1

    if args.json:
        payload = json.dumps(result.to_dict(), ensure_ascii=False, indent=2)
    else:
        payload = render(result)

    if args.output:
        with open(args.output, "w", encoding="utf-8") as fh:
            fh.write(payload + "\n")
        print(f"已写入 {args.output}", file=sys.stderr)
    else:
        print(payload)
    return 0


if __name__ == "__main__":
    sys.exit(main())

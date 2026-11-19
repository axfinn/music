import { watch } from 'vue'
import { useMediaPlayer } from './useMediaPlayer.js'

// 听满 N 首不同的歌后，温柔地提醒一次「可以请作者喝杯咖啡」。
// 设计原则：
//   - 非阻塞通知，点不点开都行，绝不弹模态框打断听歌；
//   - 提供「7 天内不提醒」与「不再提醒」两个退出选项；
//   - 默认态度是「7 天内不再主动提醒」，用户可随时在通知里改成永久关闭；
//   - 同一会话内只触发一次（armed 标记）。

const PLAYED_KEY = 'music_played_tracks'
const NUDGED_KEY = 'music_support_nudged'
const TARGET = 3

function safeGet(key) {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}
function safeSet(key, val) {
  try {
    localStorage.setItem(key, val)
  } catch {
    /* 隐私模式 / 配额满：静默降级，不影响听歌 */
  }
}
function loadPlayed() {
  try {
    const arr = JSON.parse(safeGet(PLAYED_KEY) || '[]')
    return Array.isArray(arr) ? arr : []
  } catch {
    return []
  }
}

/** 当前是否处于「被抑制」状态：永久关闭，或在某个暂缓时间窗内。 */
export function isSupportSnoozed() {
  const v = safeGet(NUDGED_KEY)
  if (!v) return false
  if (v === 'forever') return true
  const ts = Number(v)
  if (Number.isFinite(ts)) return Date.now() < ts
  return false
}

/** 暂缓 N 天后再提醒（默认 7 天）。 */
export function supportSnooze(days = 7) {
  safeSet(NUDGED_KEY, String(Date.now() + days * 86400000))
}

/** 永久不再提醒。 */
export function supportDisable() {
  safeSet(NUDGED_KEY, 'forever')
}

let armed = false // 防止同一会话里重复注册 watch / 重复触发

/**
 * 注册「听了三首歌」的提醒。
 * @param {(trackTitle:string)=>void} onReach 达到阈值时回调（由调用方决定怎么提醒）
 */
export function useSupportNudge(onReach) {
  if (armed) return
  armed = true

  const { state, currentTrack } = useMediaPlayer()
  let lastId = null

  watch(
    () => state.currentIndex,
    () => {
      // 已被用户抑制（永久关闭 / 暂缓期内）就彻底安静。
      if (isSupportSnoozed()) return

      const track = currentTrack()
      const id = track?.id
      if (!id || id === lastId) return
      lastId = id

      const played = loadPlayed()
      if (!played.includes(id)) {
        played.push(id)
        // 只保留最近 50 个，避免 localStorage 无限增长
        safeSet(PLAYED_KEY, JSON.stringify(played.slice(-50)))
      }

      if (played.length < TARGET) return

      // 达到阈值：先记下「7 天内不再主动提醒」的默认态度，
      // 用户随后可在通知里改成「不再提醒」（永久）。
      supportSnooze(7)
      try {
        onReach?.(track?.title || '')
      } catch {
        /* 提醒失败不该影响听歌 */
      }
    },
  )
}

// Command migrate 把旧一体化工具箱（devtools）里的音乐库 / 歌词库数据迁到本项目。
//
// 设计原则：**源库只读，绝不写**。
//
//   - 源库用 SQLite URI 的 mode=ro 打开，写操作会被驱动直接拒绝（不是靠代码自觉）；
//   - 目标库由 models.NewDB 打开，顺带把本项目的表结构初始化好，不用手工建表；
//   - 音频与封面文件按目录整体拷贝（<data>/music/ 与 <data>/music/covers/），
//     文件名不变，所以库里记录的 filename / cover_filename 不用改。
//
// 用法：
//
//	go run ./cmd/migrate \
//	  -source-db /path/to/devtools/data/paste.db \
//	  -source-data-dir /path/to/devtools/data \
//	  -target-db ./data/music.db \
//	  -target-data-dir ./data
//
// 加 -dry-run 先看要迁多少条，不落盘。
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/axfinn/music/backend/models"

	_ "modernc.org/sqlite"
)

// migratedTables 是要迁移的表，顺序即迁移顺序（歌词在前，音乐在后，因为音乐表有指向歌词的外键式引用）。
var migratedTables = []string{
	"lyric_songs",
	"lyric_fragments",
	"lyric_api_keys",
	"music_tracks",
}

type options struct {
	sourceDB      string
	sourceDataDir string
	targetDB      string
	targetDataDir string
	dryRun        bool
	skipFiles     bool
	overwrite     bool
}

func main() {
	var opt options
	flag.StringVar(&opt.sourceDB, "source-db", "", "旧系统的 SQLite 文件路径（必填，只读打开）")
	flag.StringVar(&opt.sourceDataDir, "source-data-dir", "", "旧系统的数据目录（里面有 music/ 子目录）；留空则只迁数据库")
	flag.StringVar(&opt.targetDB, "target-db", "./data/music.db", "本项目的 SQLite 文件路径")
	flag.StringVar(&opt.targetDataDir, "target-data-dir", "./data", "本项目的数据目录（音频与封面落在 <dir>/music/）")
	flag.BoolVar(&opt.dryRun, "dry-run", false, "只统计不写入")
	flag.BoolVar(&opt.skipFiles, "skip-files", false, "只迁数据库，不拷音频/封面文件")
	flag.BoolVar(&opt.overwrite, "overwrite", false, "目标已存在同名文件时也覆盖")
	flag.Parse()

	if err := run(opt); err != nil {
		fmt.Fprintf(os.Stderr, "迁移失败: %v\n", err)
		os.Exit(1)
	}
}

func run(opt options) error {
	if strings.TrimSpace(opt.sourceDB) == "" {
		return fmt.Errorf("必须指定 -source-db")
	}
	if _, err := os.Stat(opt.sourceDB); err != nil {
		return fmt.Errorf("源库不存在: %w", err)
	}

	// ── 源库：只读 ──
	// mode=ro 由驱动强制：任何写语句都会拿到 "attempt to write a readonly database"。
	src, err := sql.Open("sqlite", "file:"+opt.sourceDB+"?mode=ro")
	if err != nil {
		return fmt.Errorf("打开源库失败: %w", err)
	}
	defer src.Close()

	// ── 目标库：models.NewDB 顺带建表 ──
	if err := os.MkdirAll(filepath.Dir(opt.targetDB), 0o755); err != nil {
		return fmt.Errorf("创建目标库目录失败: %w", err)
	}
	tgt, err := models.NewDB(opt.targetDB)
	if err != nil {
		return fmt.Errorf("打开目标库失败: %w", err)
	}
	defer tgt.Close()

	fmt.Printf("源库: %s（只读）\n", opt.sourceDB)
	fmt.Printf("目标: %s\n\n", opt.targetDB)

	totalRows := 0
	for _, table := range migratedTables {
		n, err := copyTable(src, tgt.Conn(), table, opt.dryRun)
		if err != nil {
			return fmt.Errorf("迁移表 %s 失败: %w", table, err)
		}
		if n < 0 {
			fmt.Printf("  %-18s 源库没有这张表，跳过\n", table)
			continue
		}
		totalRows += n
		fmt.Printf("  %-18s %d 行%s\n", table, n, dryRunSuffix(opt.dryRun))
	}
	fmt.Printf("\n数据库共 %d 行%s\n", totalRows, dryRunSuffix(opt.dryRun))

	if opt.skipFiles || strings.TrimSpace(opt.sourceDataDir) == "" {
		if !opt.skipFiles {
			fmt.Println("\n未指定 -source-data-dir，跳过音频/封面文件迁移")
		}
		return nil
	}
	return copyMusicFiles(opt)
}

func dryRunSuffix(dry bool) string {
	if dry {
		return "（dry-run，未写入）"
	}
	return ""
}

// copyTable 把源表整表拷进目标表，按 id 覆盖（INSERT OR REPLACE）。
// 返回拷贝的行数；源库没有这张表时返回 -1。
func copyTable(src, tgt *sql.DB, table string, dryRun bool) (int, error) {
	exists, err := tableExists(src, table)
	if err != nil {
		return 0, err
	}
	if !exists {
		return -1, nil
	}

	rows, err := src.Query("SELECT * FROM " + quoteIdent(table))
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}

	// 只搬目标表也有的列：旧库以后加了新字段时，不会因为目标缺列而整表失败。
	targetCols, err := tableColumns(tgt, table)
	if err != nil {
		return 0, err
	}
	keep := make([]string, 0, len(cols))
	keepIdx := make([]int, 0, len(cols))
	for i, c := range cols {
		if targetCols[c] {
			keep = append(keep, c)
			keepIdx = append(keepIdx, i)
		}
	}
	if len(keep) == 0 {
		return 0, fmt.Errorf("源表与目标表没有共同字段")
	}
	dropped := len(cols) - len(keep)
	if dropped > 0 {
		fmt.Printf("  （%s 跳过 %d 个目标库没有的字段）\n", table, dropped)
	}

	insert := "INSERT OR REPLACE INTO " + quoteIdent(table) +
		" (" + strings.Join(quoteAll(keep), ", ") + ") VALUES (" +
		strings.TrimSuffix(strings.Repeat("?, ", len(keep)), ", ") + ")"

	// 目标库连接池上限为 1，事务会独占那条连接。
	// 所以 dry-run 干脆不开事务（开了又不提交也不回滚会把连接永久占住，下一张表直接死锁）。
	var tx *sql.Tx
	var stmt *sql.Stmt
	if !dryRun {
		tx, err = tgt.Begin()
		if err != nil {
			return 0, err
		}
		stmt, err = tx.Prepare(insert)
		if err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		defer stmt.Close()
	}

	count := 0
	for rows.Next() {
		all := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range all {
			ptrs[i] = &all[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			if tx != nil {
				_ = tx.Rollback()
			}
			return count, err
		}
		vals := make([]any, len(keep))
		for i, idx := range keepIdx {
			vals[i] = all[idx]
		}
		if stmt != nil {
			if _, err := stmt.Exec(vals...); err != nil {
				_ = tx.Rollback()
				return count, fmt.Errorf("写入 %s: %w", table, err)
			}
		}
		count++
	}
	if err := rows.Err(); err != nil {
		if tx != nil {
			_ = tx.Rollback()
		}
		return count, err
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return count, err
		}
	}
	return count, nil
}

// copyMusicFiles 整体拷贝 <source-data-dir>/music/ → <target-data-dir>/music/
// （covers 子目录随之带过去），文件名保持不变，库里的引用不用改。
func copyMusicFiles(opt options) error {
	srcRoot := filepath.Join(opt.sourceDataDir, "music")
	dstRoot := filepath.Join(opt.targetDataDir, "music")
	if _, err := os.Stat(srcRoot); err != nil {
		return fmt.Errorf("源音频目录不存在: %w", err)
	}
	if !opt.dryRun {
		if err := os.MkdirAll(dstRoot, 0o755); err != nil {
			return fmt.Errorf("创建目标音频目录失败: %w", err)
		}
	}

	var copied, skipped, failed int
	var totalBytes int64
	err := filepath.WalkDir(srcRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(srcRoot, path)
		if relErr != nil {
			return relErr
		}
		dst := filepath.Join(dstRoot, rel)
		if d.IsDir() {
			if !opt.dryRun {
				_ = os.MkdirAll(dst, 0o755)
			}
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		// 已存在且大小一致 → 认为已迁过，跳过（可用 -overwrite 强制覆盖）。
		if !opt.overwrite {
			if st, statErr := os.Stat(dst); statErr == nil && st.Size() == info.Size() {
				skipped++
				return nil
			}
		}
		if opt.dryRun {
			copied++
			totalBytes += info.Size()
			return nil
		}
		if err := copyFile(path, dst); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "  拷贝失败 %s: %v\n", rel, err)
			return nil // 单个文件失败不中断整体迁移
		}
		copied++
		totalBytes += info.Size()
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("\n音频/封面文件：拷贝 %d 个（%.1f MB），跳过 %d 个已存在，失败 %d 个%s\n",
		copied, float64(totalBytes)/(1024*1024), skipped, failed, dryRunSuffix(opt.dryRun))
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// tableExists 判断源库里有没有这张表。
func tableExists(db *sql.DB, table string) (bool, error) {
	var name string
	err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name = ?", table).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// tableColumns 返回目标表已有的字段集合。
func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + quoteIdent(table) + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var (
			cid        int
			name, typ  string
			notNull    int
			dfltValue  sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dfltValue, &primaryKey); err != nil {
			return nil, err
		}
		cols[name] = true
	}
	return cols, rows.Err()
}

// quoteIdent 给标识符加引号（表名/字段名都是代码里写死的常量，这里只是双保险）。
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func quoteAll(list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = quoteIdent(s)
	}
	return out
}

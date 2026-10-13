package sys

import (
	"errors"
	"os"
	"strings"
)

// RewriteLines 逐行改写文件，fn 返回 false 的行会被删掉；文件不存在时跳过
func RewriteLines(path string, fn func(line string) (string, bool)) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if l, keep := fn(line); keep {
			out = append(out, l)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o644)
}

// AppendMissing 按前缀判断，文件里还没有的行追加到末尾
func AppendMissing(path string, lines [][2]string) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var missing []string
	for _, l := range lines {
		if !strings.Contains(string(data), l[0]) {
			missing = append(missing, l[1])
		}
	}
	if len(missing) == 0 {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(strings.Join(missing, "\n") + "\n")
	return err
}

const fstab = "/etc/fstab"

// EditFstab 删掉 keep 返回 false 的条目后追加新行，注释和空行原样保留
func EditFstab(keep func(fields []string) bool, add string) error {
	err := RewriteLines(fstab, func(line string) (string, bool) {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			return line, true
		}
		return line, keep(fields)
	})
	if err != nil || add == "" {
		return err
	}
	return AppendMissing(fstab, [][2]string{{add, add}})
}

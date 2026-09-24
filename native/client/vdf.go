package nativeclient

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// 这里只实现 Steam 用到的那个子集：带引号的键值、花括号嵌套、// 注释。
// 我们不写入这种格式，也不需要保留键的顺序之外的任何信息，所以解析成一棵
// map[string]any 就够了，嵌套块是 map，叶子是 string。

type vdfParser struct {
	src []byte
	pos int
}

func parseVDF(src []byte) (map[string]any, error) {
	p := &vdfParser{src: src}
	return p.parseEntries(false)
}

func (p *vdfParser) skipSpace() {
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			p.pos++
		case c == '/' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '/':
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

// parseObject 读取一个花括号块，调用方已经确认下一个非空白字符是左花括号。
func (p *vdfParser) parseObject() (map[string]any, error) {
	if p.pos >= len(p.src) || p.src[p.pos] != '{' {
		return nil, fmt.Errorf("nativeclient: VDF 缺少 {（偏移 %d）", p.pos)
	}
	p.pos++
	return p.parseEntries(true)
}

// parseEntries 读取键值对；expectClose 为真时以右花括号结束，为假时读到文件末尾。
//
// Steam 的 libraryfolders.vdf 顶层就是若干 "key" { … }，所以这两种结束条件是同一个
// 循环的两个出口，没必要写成两份解析。
func (p *vdfParser) parseEntries(expectClose bool) (map[string]any, error) {
	object := map[string]any{}
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			if !expectClose {
				return object, nil
			}
			return nil, fmt.Errorf("nativeclient: VDF 在对象结束前截断")
		}
		if p.src[p.pos] == '}' {
			if !expectClose {
				return nil, fmt.Errorf("nativeclient: VDF 在偏移 %d 处出现多余的 }", p.pos)
			}
			p.pos++
			return object, nil
		}
		key, err := p.parseToken()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("nativeclient: VDF 在 %q 的值前截断", key)
		}
		if p.src[p.pos] == '{' {
			value, err := p.parseObject()
			if err != nil {
				return nil, err
			}
			object[key] = value
			continue
		}
		value, err := p.parseToken()
		if err != nil {
			return nil, err
		}
		object[key] = value
	}
}

func (p *vdfParser) parseToken() (string, error) {
	if p.pos >= len(p.src) {
		return "", fmt.Errorf("nativeclient: VDF 意外结束")
	}
	if p.src[p.pos] == '"' {
		p.pos++
		var builder strings.Builder
		for p.pos < len(p.src) {
			c := p.src[p.pos]
			if c == '\\' && p.pos+1 < len(p.src) {
				builder.WriteByte(p.src[p.pos+1])
				p.pos += 2
				continue
			}
			if c == '"' {
				p.pos++
				return builder.String(), nil
			}
			builder.WriteByte(c)
			p.pos++
		}
		return "", fmt.Errorf("nativeclient: VDF 字符串没有闭合引号")
	}
	start := p.pos
	for p.pos < len(p.src) {
		c := rune(p.src[p.pos])
		if unicode.IsSpace(c) || c == '{' || c == '}' || c == '"' {
			break
		}
		p.pos++
	}
	if start == p.pos {
		return "", fmt.Errorf("nativeclient: VDF 在偏移 %d 处出现无法解析的字符 %q", p.pos, p.src[p.pos])
	}
	return string(p.src[start:p.pos]), nil
}

// libraryPaths 从 libraryfolders.vdf 里取出所有库目录。
//
// 新旧两种 Steam 格式都要认：新版是 libraryfolders/<n>/path，老版直接把
// libraryfolders/<n> 写成路径字符串。
func libraryPaths(doc map[string]any) []string {
	raw, ok := doc["libraryfolders"]
	if !ok {
		if raw, ok = doc["LibraryFolders"]; !ok {
			return nil
		}
	}
	folders, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(folders))
	for key := range folders {
		keys = append(keys, key)
	}
	// 数字键按数值排序，其余按字典序，保证同样的输入得到同样的输出。
	sortNumericKeys(keys)
	var paths []string
	for _, key := range keys {
		switch value := folders[key].(type) {
		case map[string]any:
			if entry, ok := value["path"].(string); ok && entry != "" {
				paths = append(paths, entry)
			}
		case string:
			if value != "" {
				paths = append(paths, value)
			}
		}
	}
	return paths
}

func sortNumericKeys(keys []string) {
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && lessKey(keys[j], keys[j-1]); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
}

func lessKey(a, b string) bool {
	na, errA := strconv.Atoi(a)
	nb, errB := strconv.Atoi(b)
	if errA == nil && errB == nil {
		return na < nb
	}
	return a < b
}

package project

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"

	"wbo/internal/ir"
)

func Compile(l *Loaded, allowUnresolved bool) ([]byte, error) {
	if l.HasErrors() {
		return nil, fmt.Errorf("存在编译错误")
	}
	if len(l.Cards) > 0 && len(l.Tests) > 0 {
		return nil, fmt.Errorf("卡牌和测试不能混编")
	}
	if len(l.Unresolved) > 0 && !allowUnresolved {
		return nil, fmt.Errorf("存在未解析卡牌引用")
	}
	if len(l.Cards) > 0 {
		pack, err := compileCardsTyped(l)
		if err != nil {
			return nil, err
		}
		if allowUnresolved {
			pack.UnresolvedReferences = unresolvedIDs(l)
		}
		return ir.EncodeCardPack(pack)
	} else if len(l.Tests) > 0 {
		pack, err := compileTestsTyped(l)
		if err != nil {
			return nil, err
		}
		if allowUnresolved {
			pack.UnresolvedReferences = unresolvedIDs(l)
		}
		return ir.EncodeTestPack(pack)
	} else {
		return nil, fmt.Errorf("没有可编译的输入")
	}
}

func unresolvedIDs(l *Loaded) []string {
	ids := make([]string, 0, len(l.Unresolved))
	for id := range l.Unresolved {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// BuildRuntimePacks proves the runtime boundary by serializing both lowered
// packages and strictly decoding the bytes before returning typed IR.
func BuildRuntimePacks(l *Loaded) (*ir.CardPack, *ir.TestPack, error) {
	if l.HasErrors() {
		return nil, nil, fmt.Errorf("存在编译错误")
	}
	if len(l.Unresolved) != 0 {
		return nil, nil, fmt.Errorf("存在未解析卡牌引用")
	}
	cardsLoaded := &Loaded{Root: l.Root, Cards: append(append([]*Card{}, l.Cards...), l.Dependencies...)}
	cardIR, err := compileCardsTyped(cardsLoaded)
	if err != nil {
		return nil, nil, err
	}
	cardBytes, err := ir.EncodeCardPack(cardIR)
	if err != nil {
		return nil, nil, err
	}
	cardPack, err := ir.DecodeCardPack(cardBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("decode runtime card pack: %w", err)
	}
	testIR, err := compileTestsTyped(l)
	if err != nil {
		return nil, nil, err
	}
	testBytes, err := ir.EncodeTestPack(testIR)
	if err != nil {
		return nil, nil, err
	}
	testPack, err := ir.DecodeTestPack(testBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("decode runtime test pack: %w", err)
	}
	if err := ir.ValidateRuntimePacks(cardPack, testPack); err != nil {
		return nil, nil, fmt.Errorf("link runtime packs: %w", err)
	}
	return cardPack, testPack, nil
}

func sourcePath(l *Loaded, path string) string {
	p, err := filepath.Rel(l.Root, path)
	if err != nil {
		p = path
	}
	return filepath.ToSlash(p)
}
func mustInt(s string) int { n, _ := strconv.Atoi(s); return n }
func unique(v []string) []string {
	seen := map[string]bool{}
	r := []string{}
	for _, x := range v {
		if !seen[x] {
			seen[x] = true
			r = append(r, x)
		}
	}
	return r
}

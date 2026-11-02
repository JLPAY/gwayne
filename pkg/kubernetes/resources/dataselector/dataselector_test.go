package dataselector

import (
	"testing"

	"github.com/JLPAY/gwayne/pkg/pagequery"
)

// fakeCell 仅实现 name/status 两个属性，用于验证排序链路本身
type fakeCell struct {
	name   string
	status string
}

func (c fakeCell) GetProperty(name PropertyName) ComparableValue {
	switch name {
	case NameProperty:
		return StdComparableString(c.name)
	case StatusProperty:
		return StdComparableString(c.status)
	default:
		return nil
	}
}

func buildCells() []DataCell {
	return []DataCell{
		fakeCell{"a", "Running"},
		fakeCell{"b", "CrashLoopBackOff"},
		fakeCell{"c", "Terminating"},
		fakeCell{"d", "Pending"},
		fakeCell{"e", "Running"},
	}
}

func selectedNames(page *pagequery.Page) []string {
	list, ok := page.List.([]DataCell)
	if !ok {
		panic("unexpected page list type")
	}
	out := make([]string, 0, len(list))
	for _, c := range list {
		out = append(out, string(c.GetProperty(NameProperty).(StdComparableString)))
	}
	return out
}

func assertOrder(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: 长度不符 got=%v want=%v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: 顺序不符 got=%v want=%v", label, got, want)
		}
	}
}

// TestSortByStatus 验证按 status 排序：先按状态字典序，状态相同的再按 name 升/降序兜底
func TestSortByStatus(t *testing.T) {
	asc := selectedNames(DataSelectPage(buildCells(), &pagequery.QueryParam{PageNo: 1, PageSize: 10, Sortby: "status"}))
	assertOrder(t, "status", asc, []string{"b", "d", "a", "e", "c"})

	desc := selectedNames(DataSelectPage(buildCells(), &pagequery.QueryParam{PageNo: 1, PageSize: 10, Sortby: "-status"}))
	assertOrder(t, "-status", desc, []string{"c", "e", "a", "d", "b"})
}

// TestSortByStatusPaged 同状态记录分页时顺序必须稳定，不能来回跳
func TestSortByStatusPaged(t *testing.T) {
	first := selectedNames(DataSelectPage(buildCells(), &pagequery.QueryParam{PageNo: 1, PageSize: 2, Sortby: "status"}))
	second := selectedNames(DataSelectPage(buildCells(), &pagequery.QueryParam{PageNo: 2, PageSize: 2, Sortby: "status"}))
	assertOrder(t, "page1", first, []string{"b", "d"})
	assertOrder(t, "page2", second, []string{"a", "e"})
}

// TestSortByUnknownProperty 未支持的属性不改变顺序，也不能 panic
func TestSortByUnknownProperty(t *testing.T) {
	got := selectedNames(DataSelectPage(buildCells(), &pagequery.QueryParam{PageNo: 1, PageSize: 10, Sortby: "no-such-field"}))
	assertOrder(t, "unknown", got, []string{"a", "b", "c", "d", "e"})
}

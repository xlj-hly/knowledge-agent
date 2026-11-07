package repository

import (
	"reflect"
	"testing"
)

func TestHanBigrams(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		want        []string
	}{
		{"natural question", "审批人是谁？", []string{"审批", "批人", "人是", "是谁"}},
		{"deduplicated", "哈哈哈，哈哈", []string{"哈哈"}},
		{"boundaries", "航星、计划 API接口 A股", []string{"航星", "计划", "接口"}},
		{"single character", "沐", []string{}},
		{"English", "reimbursement receipt", []string{}},
		{"supplementary Han", "𠀀航星", []string{"𠀀航", "航星"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hanBigrams(tc.input); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

package main

import (
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	type args struct {
		text        string
		delimiter   string
		maxNumChars int
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{"00", args{"1 2 3 4 5 6 ", " ", 20}, []string{"1 2 3 4 5 6 "}},
		{"01", args{"1 2 3 4 5 6 ", " ", 2}, []string{"1 ", "2 ", "3 ", "4 ", "5 ", "6 "}},
		{"02", args{"123 2 3 4 5 6 ", " ", 2}, []string{"123 ", "2 ", "3 ", "4 ", "5 ", "6 "}},
		{"03", args{"123 1234 12345 123456 1234567 12345678 ", " ", 9}, []string{"123 1234 ", "12345 ", "123456 ", "1234567 ", "12345678 "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Split(tt.args.text, tt.args.delimiter, tt.args.maxNumChars); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Split() = %v, want %v", got, tt.want)
			}
		})
	}
}

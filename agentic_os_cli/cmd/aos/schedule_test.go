package main

import (
	"reflect"
	"testing"

	"github.com/ultrakorne/aos_cli/internal/scheduler"
)

func TestParseDays(t *testing.T) {
	cases := []struct {
		in      string
		want    []scheduler.Weekday
		wantErr bool
	}{
		{"mon", []scheduler.Weekday{scheduler.Mon}, false},
		{"mon,tue,wed,thu,fri", []scheduler.Weekday{scheduler.Mon, scheduler.Tue, scheduler.Wed, scheduler.Thu, scheduler.Fri}, false},
		{"mon-fri", []scheduler.Weekday{scheduler.Mon, scheduler.Tue, scheduler.Wed, scheduler.Thu, scheduler.Fri}, false},
		{"sun-sat", []scheduler.Weekday{scheduler.Sun, scheduler.Mon, scheduler.Tue, scheduler.Wed, scheduler.Thu, scheduler.Fri, scheduler.Sat}, false},
		{" Mon , Wed , Fri ", []scheduler.Weekday{scheduler.Mon, scheduler.Wed, scheduler.Fri}, false},
		{"", nil, true},
		{"bogus", nil, true},
		{"mon,bogus", nil, true},
		{"fri-mon", nil, true},   // reverse range rejected — sun=0..sat=6 ordering
		{"mon-fri,sat", nil, true}, // range + comma not supported
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseDays(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

package apptypes

import (
	"reflect"
	"testing"

	"github.com/filipemolina/chore-crusher/src/store"
)

// fillNonZero sets every field reachable from v to a recognisable non-zero
// value, recursing into nested structs and allocating pointers so a *int64
// becomes a pointer to a non-zero int64 rather than nil. It is driven by
// reflection on purpose: a field added to a store struct is covered here the
// moment it is declared, with no test to remember to update.
func fillNonZero(t *testing.T, v reflect.Value, name string) {
	t.Helper()
	switch v.Kind() {
	case reflect.String:
		v.SetString("x-" + name)
	case reflect.Int, reflect.Int32, reflect.Int64:
		v.SetInt(7)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Pointer:
		p := reflect.New(v.Type().Elem())
		fillNonZero(t, p.Elem(), name)
		v.Set(p)
	case reflect.Struct:
		for i := range v.NumField() {
			fillNonZero(t, v.Field(i), name+"."+v.Type().Field(i).Name)
		}
	default:
		t.Fatalf("fillNonZero: unhandled kind %s at %s — extend this helper", v.Kind(), name)
	}
}

// assertNoZeroField reports every field reachable from v that the converter
// left at its zero value, recursing into nested structs so a half-copied
// embedded struct is caught rather than passing on its non-zero siblings.
func assertNoZeroField(t *testing.T, v reflect.Value, name string) {
	t.Helper()
	if v.Kind() == reflect.Struct {
		for i := range v.NumField() {
			assertNoZeroField(t, v.Field(i), name+"."+v.Type().Field(i).Name)
		}
		return
	}
	if v.IsZero() {
		t.Errorf("%s was left at its zero value: the converter does not copy it, "+
			"so the TUI reads it empty no matter what the store holds", name)
	}
}

// TestFromStoreConvertersCopyEveryField guards the boundary that §6.4 names
// as a silent failure mode: the store -> apptypes conversions are functions
// rather than type aliases so a field cannot leak between the layers, but
// nothing made that promise enforceable. A field added to a store struct AND to its apptypes
// mirror but forgotten in the converter compiles fine and reads empty in
// every component — exactly how Assignee, AssignedAt and Priority could have
// arrived broken in step 2.
//
// Every store-side field is filled with a non-zero value, so any destination
// field the converter does not write shows up as a zero here. The assertion
// runs on the destination only: an apptypes mirror is free to expose fewer
// fields than its store row (List does — it drops CreatedBy and
// CommentsDisabled), but every field it does expose must come from somewhere.
func TestFromStoreConvertersCopyEveryField(t *testing.T) {
	cases := []struct {
		name    string
		zero    any
		convert func(reflect.Value) any
	}{
		{"FromStore", store.Task{}, func(v reflect.Value) any {
			return FromStore(v.Interface().(store.Task))
		}},
		{"FromStoreList", store.List{}, func(v reflect.Value) any {
			return FromStoreList(v.Interface().(store.List))
		}},
		{"FromStoreComment", store.Comment{}, func(v reflect.Value) any {
			return FromStoreComment(v.Interface().(store.Comment))
		}},
		{"FromStoreActivity", store.AgentActivity{}, func(v reflect.Value) any {
			return FromStoreActivity(v.Interface().(store.AgentActivity))
		}},
		{"FromStoreLists", store.ListSummary{}, func(v reflect.Value) any {
			got := FromStoreLists([]store.ListSummary{v.Interface().(store.ListSummary)})
			return got[0]
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := reflect.New(reflect.TypeOf(tc.zero)).Elem()
			fillNonZero(t, src, tc.name+" input")

			got := reflect.ValueOf(tc.convert(src))
			assertNoZeroField(t, got, tc.name+" result")
		})
	}
}

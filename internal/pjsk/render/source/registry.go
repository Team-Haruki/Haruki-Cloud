package source

import (
	"reflect"

	renderregion "haruki-cloud/internal/pjsk/render/region"
)

type Regioned interface {
	DefaultRegion() renderregion.Value
}

type Registry[T Regioned] struct {
	defaultRegion renderregion.Value
	sources       map[renderregion.Value]T
	order         []T
}

func NewRegistry[T Regioned](defaultRegion renderregion.Value) *Registry[T] {
	return &Registry[T]{
		defaultRegion: renderregion.Normalize(defaultRegion.String()),
		sources:       make(map[renderregion.Value]T),
	}
}

func NormalizeRegion(value renderregion.Value) renderregion.Value {
	return renderregion.Normalize(value.String())
}

func (r *Registry[T]) RegisterSource(src T) {
	if isNilValue(src) {
		return
	}

	region := NormalizeRegion(src.DefaultRegion())
	if region.IsZero() {
		region = r.ResolveRegion(renderregion.Unknown)
	}
	if _, ok := r.sources[region]; !ok {
		r.order = append(r.order, src)
	}
	r.sources[region] = src
	if r.defaultRegion.IsZero() {
		r.defaultRegion = region
	}
}

func (r *Registry[T]) ResolveRegion(requested renderregion.Value) renderregion.Value {
	if normalized := NormalizeRegion(requested); !normalized.IsZero() {
		return normalized
	}
	if !r.defaultRegion.IsZero() {
		return r.defaultRegion
	}
	for _, src := range r.order {
		if isNilValue(src) {
			continue
		}
		if normalized := NormalizeRegion(src.DefaultRegion()); !normalized.IsZero() {
			return normalized
		}
	}
	return renderregion.JP
}

func (r *Registry[T]) SourceForRegion(region renderregion.Value) (T, bool) {
	normalized := NormalizeRegion(region)
	if !normalized.IsZero() {
		if src, ok := r.sources[normalized]; ok && !isNilValue(src) {
			return src, true
		}
		var zero T
		return zero, false
	}

	normalized = r.ResolveRegion(region)
	if src, ok := r.sources[normalized]; ok && !isNilValue(src) {
		return src, true
	}
	for _, src := range r.order {
		if !isNilValue(src) {
			return src, true
		}
	}

	var zero T
	return zero, false
}

func (r *Registry[T]) OrderedSources() []T {
	if r == nil || len(r.order) == 0 {
		return nil
	}
	return append([]T(nil), r.order...)
}

func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

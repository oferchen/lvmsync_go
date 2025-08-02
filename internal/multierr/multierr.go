package multierr

import "strings"

type multiError struct {
	errs []error
}

func (m *multiError) Error() string {
	if len(m.errs) == 0 {
		return ""
	}
	if len(m.errs) == 1 {
		return m.errs[0].Error()
	}
	var b strings.Builder
	for i, e := range m.errs {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(e.Error())
	}
	return b.String()
}

func (m *multiError) Errors() []error {
	return append([]error(nil), m.errs...)
}

func Append(err error, errs ...error) error {
	all := append([]error{err}, errs...)
	return Combine(all...)
}

func Combine(errs ...error) error {
	var out []error
	for _, e := range errs {
		if e == nil {
			continue
		}
		if me, ok := e.(*multiError); ok {
			out = append(out, me.errs...)
		} else {
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return nil
	}
	if len(out) == 1 {
		return out[0]
	}
	return &multiError{errs: out}
}

func Errors(err error) []error {
	if err == nil {
		return nil
	}
	if me, ok := err.(*multiError); ok {
		return me.Errors()
	}
	return []error{err}
}

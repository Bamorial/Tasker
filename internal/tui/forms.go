package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
)

type fieldKind int

const (
	fieldText fieldKind = iota
	fieldSelect
)

type formField struct {
	Key         string
	Label       string
	Kind        fieldKind
	Input       textinput.Model
	Options     []string
	OptionIndex int
	Help        string
}

func newTextField(key, label, value, placeholder, help string) formField {
	input := textinput.New()
	input.Prompt = ""
	input.SetValue(value)
	input.Placeholder = placeholder
	input.CharLimit = 256
	input.Width = 48
	return formField{
		Key:   key,
		Label: label,
		Kind:  fieldText,
		Input: input,
		Help:  help,
	}
}

func newSelectField(key, label string, options []string, current, help string) formField {
	index := 0
	for i, option := range options {
		if option == current {
			index = i
			break
		}
	}
	return formField{
		Key:         key,
		Label:       label,
		Kind:        fieldSelect,
		Options:     options,
		OptionIndex: index,
		Help:        help,
	}
}

func (f *formField) Value() string {
	switch f.Kind {
	case fieldSelect:
		if len(f.Options) == 0 {
			return ""
		}
		return f.Options[f.OptionIndex]
	default:
		return strings.TrimSpace(f.Input.Value())
	}
}

func (f *formField) SetFocus(focused bool) {
	if f.Kind != fieldText {
		return
	}
	if focused {
		f.Input.Focus()
		return
	}
	f.Input.Blur()
}

func (f *formField) Cycle(delta int) {
	if f.Kind != fieldSelect || len(f.Options) == 0 {
		return
	}
	size := len(f.Options)
	f.OptionIndex = (f.OptionIndex + delta + size) % size
}

type formModal struct {
	Kind        modalKind
	Title       string
	SubmitLabel string
	Fields      []formField
	Focus       int
}

func (f *formModal) Values() map[string]string {
	values := make(map[string]string, len(f.Fields))
	for i := range f.Fields {
		values[f.Fields[i].Key] = f.Fields[i].Value()
	}
	return values
}

func (f *formModal) FocusCurrent() {
	for i := range f.Fields {
		f.Fields[i].SetFocus(i == f.Focus)
	}
}

func (f *formModal) Next() {
	if len(f.Fields) == 0 {
		return
	}
	f.Focus = (f.Focus + 1) % len(f.Fields)
	f.FocusCurrent()
}

func (f *formModal) Prev() {
	if len(f.Fields) == 0 {
		return
	}
	f.Focus = (f.Focus - 1 + len(f.Fields)) % len(f.Fields)
	f.FocusCurrent()
}

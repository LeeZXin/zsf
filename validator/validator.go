package validator

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

var (
	ErrInvalid        = errors.New("invalid value")
	ErrZeroValue      = errors.New("nil value")
	ErrMin            = errors.New("less than min")
	ErrMax            = errors.New("greater than max")
	ErrLen            = errors.New("invalid length")
	ErrRegexp         = errors.New("regular expression mismatch")
	ErrUnsupported    = errors.New("unsupported type")
	ErrBadParameter   = errors.New("bad parameter")
	ErrCannotValidate = errors.New("cannot validate unexported struct")
)

type ErrorMap map[string]ErrorArray

type Bindings map[string]string

func (err ErrorMap) Error() string {
	var b bytes.Buffer
	for _, errs := range err {
		if len(errs) > 0 {
			b.WriteString(errs.Error())
			b.WriteString(";")
		}
	}
	return strings.TrimSuffix(b.String(), ";")
}

type ErrorArray []error

func (err ErrorArray) Error() string {
	var b bytes.Buffer
	for _, errs := range err {
		b.WriteString(fmt.Sprintf("%s,", errs.Error()))
	}
	errs := b.String()
	return strings.TrimSuffix(errs, ",")
}

type ValidationFunc func(v interface{}, param string, bindings map[string]string) error

type Validator struct {
	validationFuncs map[string]ValidationFunc
	tagName         string
	translator      Translator
}

var defaultValidator = NewValidator()

func NewValidator() *Validator {
	return &Validator{
		tagName: "validate",
		validationFuncs: map[string]ValidationFunc{
			"nonzero":  nonzero,
			"len":      length,
			"min":      min,
			"max":      max,
			"regexp":   regex,
			"nonnil":   nonnil,
			"notBlank": notBlank,
			"email":    email,
		},
		translator: Translator{
			"nonzero":  "不能为nil",
			"len":      "长度必须等于{len}",
			"min":      "不能小于{min}",
			"max":      "不能大于{max}",
			"regexp":   "格式不正确",
			"nonnil":   "不能为nil",
			"notBlank": "不能为空",
			"email":    "邮箱格式不正确",
		},
	}
}

func (mv *Validator) SetTag(tag string) {
	mv.tagName = tag
}

func (mv *Validator) SetTranslator(translator map[string]string) {
	mv.translator = translator
}

func (mv *Validator) copy() *Validator {
	newFuncs := map[string]ValidationFunc{}
	for k, f := range mv.validationFuncs {
		newFuncs[k] = f
	}
	return &Validator{
		tagName:         mv.tagName,
		validationFuncs: newFuncs,
		translator:      mv.translator,
	}
}

func SetValidationFunc(name string, vf ValidationFunc) {
	defaultValidator.SetValidationFunc(name, vf)
}

func (mv *Validator) SetValidationFunc(name string, vf ValidationFunc) {
	if name == "" {
		return
	}
	if vf == nil {
		delete(mv.validationFuncs, name)
	} else {
		mv.validationFuncs[name] = vf
	}
}

func Validate(v interface{}) error {
	return defaultValidator.Validate(v)
}

func (mv *Validator) Validate(v interface{}) error {
	m := make(ErrorMap)
	mv.deepValidateCollection(reflect.ValueOf(v), m, func() string {
		return ""
	})
	if len(m) > 0 {
		for k, arr := range m {
			for _, err := range arr {
				te, ok := err.(*TemplateErr)
				if ok {
					if te.VarName == "" {
						te.VarName = k
					}
				}
			}
		}
		return m
	}
	return nil
}

func (mv *Validator) validateStruct(sv reflect.Value, m ErrorMap) error {
	kind := sv.Kind()
	if (kind == reflect.Ptr || kind == reflect.Interface) && !sv.IsNil() {
		return mv.validateStruct(sv.Elem(), m)
	}
	if kind != reflect.Struct && kind != reflect.Interface {
		return ErrUnsupported
	}
	st := sv.Type()
	nfields := st.NumField()
	for i := 0; i < nfields; i++ {
		if err := mv.validateField(st.Field(i), sv.Field(i), m); err != nil {
			return err
		}
	}
	return nil
}

func (mv *Validator) validateField(fieldDef reflect.StructField, fieldVal reflect.Value, m ErrorMap) error {
	tag := fieldDef.Tag.Get(mv.tagName)
	if tag == "-" {
		return nil
	}
	for (fieldVal.Kind() == reflect.Ptr || fieldVal.Kind() == reflect.Interface) && !fieldVal.IsNil() {
		fieldVal = fieldVal.Elem()
	}
	if !fieldDef.Anonymous && fieldDef.PkgPath != "" {
		return nil
	}

	var errs ErrorArray
	if tag != "" {
		var err error
		if fieldDef.PkgPath != "" {
			err = ErrCannotValidate
		} else {
			err = mv.validValue(fieldVal, tag)
		}
		if errarr, ok := err.(ErrorArray); ok {
			errs = errarr
		} else if err != nil {
			errs = ErrorArray{err}
		}
	}
	fn := fieldDef.Name
	mv.deepValidateCollection(fieldVal, m, func() string {
		return fn
	})
	if len(errs) > 0 {
		m[fn] = errs
	}
	return nil
}

func (mv *Validator) deepValidateCollection(f reflect.Value, m ErrorMap, fnameFn func() string) {
	switch f.Kind() {
	case reflect.Interface, reflect.Ptr:
		if f.IsNil() {
			return
		}
		mv.deepValidateCollection(f.Elem(), m, fnameFn)
	case reflect.Struct:
		subm := make(ErrorMap)
		err := mv.validateStruct(f, subm)
		parentName := fnameFn()
		if err != nil {
			m[parentName] = ErrorArray{err}
		}
		for j, k := range subm {
			keyName := j
			if parentName != "" {
				keyName = parentName + "." + keyName
			}
			m[keyName] = k
		}
	case reflect.Array, reflect.Slice:
		switch f.Type().Elem().Kind() {
		case reflect.Struct, reflect.Interface, reflect.Ptr, reflect.Map, reflect.Array, reflect.Slice:
			for i := 0; i < f.Len(); i++ {
				mv.deepValidateCollection(f.Index(i), m, func() string {
					return fmt.Sprintf("%s[%d]", fnameFn(), i)
				})
			}
		}
	case reflect.Map:
		for _, key := range f.MapKeys() {
			mv.deepValidateCollection(key, m, func() string {
				return fmt.Sprintf("%s[%+v](key)", fnameFn(), key.Interface())
			})
			value := f.MapIndex(key)
			mv.deepValidateCollection(value, m, func() string {
				return fmt.Sprintf("%s[%+v](value)", fnameFn(), key.Interface())
			})
		}
	}
}

func (mv *Validator) validValue(v reflect.Value, tags string) error {
	if v.Kind() == reflect.Invalid {
		return mv.validateVar(nil, tags)
	}
	return mv.validateVar(v.Interface(), tags)
}

func (mv *Validator) validateVar(v interface{}, tag string) error {
	tags, name, err := mv.parseTags(tag)
	if err != nil {
		return err
	}
	errs := make(ErrorArray, 0, len(tags))
	for _, t := range tags {
		bindings := make(Bindings)
		if err := t.Fn(v, t.Param, bindings); err != nil {
			format, ok := mv.translator[t.Name]
			if !ok {
				format = err.Error()
			}
			errs = append(errs, &TemplateErr{
				VarName:  name,
				Format:   format,
				Bindings: bindings,
			})
		}
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

type tag struct {
	Name  string         // name of the tag
	Fn    ValidationFunc // validation function to call
	Param string         // parameter to send to the validation function
}

var sepPattern *regexp.Regexp = regexp.MustCompile(`((?:^|[^\\])(?:\\\\)*),`)

func splitUnescapedComma(str string) []string {
	ret := []string{}
	indexes := sepPattern.FindAllStringIndex(str, -1)
	last := 0
	for _, is := range indexes {
		ret = append(ret, str[last:is[1]-1])
		last = is[1]
	}
	ret = append(ret, str[last:])
	return ret
}

func (mv *Validator) parseTags(t string) ([]tag, string, error) {
	tl := splitUnescapedComma(t)
	tags := make([]tag, 0, len(tl))
	name := ""
	for _, i := range tl {
		i = strings.Replace(i, `\,`, ",", -1)
		tg := tag{}
		v := strings.SplitN(i, "=", 2)
		tg.Name = strings.Trim(v[0], " ")
		if tg.Name == "" {
			continue
		}
		if len(v) > 1 {
			tg.Param = strings.Trim(v[1], " ")
		}
		if tg.Name == "alias" {
			name = tg.Param
			continue
		}
		fn, found := mv.validationFuncs[tg.Name]
		if !found {
			continue
		}
		tg.Fn = fn
		tags = append(tags, tg)
	}
	return tags, name, nil
}

func parseName(tag string) string {
	if tag == "" {
		return ""
	}
	name := strings.SplitN(tag, ",", 2)[0]
	if name == "-" {
		return ""
	}
	return name
}

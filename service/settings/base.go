package settings

import (
	"errors"
	"reflect"
)

type baseModel struct {
	// isDirty bool
	loaded bool
	name   string
	store  *settingsStore
	onSave func(model Model)
}

func (m *baseModel) save(model Model) error {
	// if !m.isDirty {
	// 	return nil
	// }

	if m.store == nil {
		return errors.New("model is detached")
	}

	if err := m.store.save(m.name, model); err != nil {
		return err
	}

	// m.isDirty = false
	if m.onSave != nil {
		m.onSave(model)
	}

	return nil
}

// loadPersisted loads only persisted values from storage without merging defaults.
// Used by Validate() to compare current values against persisted ones.
func (m *baseModel) loadPersisted(model Model) error {
	return m.load(model, nil)
}

func (m *baseModel) load(model Model, defaultVal Model) error {
	if m.store == nil {
		return errors.New("model is detached")
	}

	loaded, err := m.store.load(m.name, model)
	if err != nil {
		return err
	}
	m.loaded = loaded

	if defaultVal != nil {
		if err := mergeModel(defaultVal, model); err != nil {
			return err
		}
	}

	// m.isDirty = false
	return nil
}

func mergeModel(src, dest Model) error {
	if reflect.TypeOf(src) != reflect.TypeOf(dest) {
		return errors.New("model types unmacthed")
	}

	srcVal := reflect.ValueOf(src).Elem()
	destVal := reflect.ValueOf(dest).Elem()

	for i := 0; i < destVal.NumField(); i++ {
		field := destVal.Field(i)
		if !field.CanSet() {
			continue
		}

		srcTypeField := srcVal.Type().Field(i)
		if _, skip := srcTypeField.Tag.Lookup("skipMerge"); skip {
			continue
		}

		if reflect.Zero(field.Type()).Interface() == field.Interface() {
			srcField := srcVal.Field(i)
			fieldVal := srcField.Interface()
			if err := setFieldValue(field, fieldVal); err != nil {
				return err
			}
		}
	}

	return nil
}

func modelEqual(model1, model2 Model) bool {
	if reflect.TypeOf(model1) != reflect.TypeOf(model2) {
		return false
	}

	m1Val := reflect.ValueOf(model1).Elem()
	m2Val := reflect.ValueOf(model2).Elem()

	for i := 0; i < m1Val.NumField(); i++ {
		field := m1Val.Field(i)
		if !field.CanSet() {
			continue
		}

		if m1Val.Field(i).Interface() != m2Val.Field(i).Interface() {
			return false
		}
	}

	return true
}

// only some primitive types are supported
func setFieldValue(field reflect.Value, val any) error {
	if !field.CanSet() || val == nil {
		return nil
	}

	v := reflect.ValueOf(val)
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			// Stop if we hit a nil pointer/interface
			return nil
		}
		v = v.Elem()
	}
	// Check if we ended up with a valid, concrete value.
	if !v.IsValid() {
		return nil
	}

	if v.Type().AssignableTo(field.Type()) {
		field.Set(v)
		return nil
	}

	if v.Type().ConvertibleTo(field.Type()) {
		field.Set(v.Convert(field.Type()))
		return nil
	}

	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Kind() < reflect.Int || v.Kind() > reflect.Int64 {
			return errors.New("value is not an int")
		}
		field.SetInt(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v.Kind() < reflect.Uint || v.Kind() > reflect.Uint64 {
			return errors.New("value is not a uint")
		}
		field.SetUint(v.Uint())
	case reflect.Float32, reflect.Float64:
		if v.Kind() != reflect.Float32 && v.Kind() != reflect.Float64 {
			return errors.New("value is not a float")
		}
		field.SetFloat(v.Float())
	case reflect.String:
		if v.Kind() != reflect.String {
			return errors.New("value is not a string")
		}
		field.SetString(v.String())
	case reflect.Bool:
		if v.Kind() != reflect.Bool {
			return errors.New("value is not a bool")
		}
		field.SetBool(v.Bool())
	default:
		return errors.New("can not set value of kind: " + field.Kind().String())
	}
	return nil
}

// convert to base type
// func baseTypeVal(val any) any {
// 	return reflect.ValueOf(val).Convert(reflect.TypeOf(int(0))).Interface().(int)

// }

func isModel(m any) bool {
	t := reflect.ValueOf(m).Elem()
	convertable := false
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		if field.Type().ConvertibleTo(reflect.TypeOf(&baseModel{})) {
			convertable = true
			break
		}
	}

	return convertable
}

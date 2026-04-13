package settings

import (
	"errors"
	"reflect"

	"looz.ws/typstify/utils"
)

type baseModel struct {
	// isDirty bool
	loaded bool
	bucket *utils.Bucket[utils.SKey, any]
	onSave func()
}

func (m *baseModel) save(model Model) error {
	// if !m.isDirty {
	// 	return nil
	// }

	if m.bucket == nil {
		return errors.New("model is detached")
	}

	t := reflect.ValueOf(model).Elem()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.CanSet() {
			continue
		}

		fieldVal := field.Interface()

		typeField := t.Type().Field(i)
		keyName := typeField.Tag.Get("key")
		err := m.bucket.Save(utils.SKey(keyName), &fieldVal)
		if err != nil {
			return err
		}
	}

	// m.isDirty = false
	if m.onSave != nil {
		m.onSave()
	}

	return nil
}

// loadPersisted loads only persisted values from storage without merging defaults.
// Used by Validate() to compare current values against persisted ones.
func (m *baseModel) loadPersisted(model Model) error {
	m.load(model, nil)
	return nil
}

func (m *baseModel) load(model Model, defaultVal Model) error {
	if m.bucket == nil {
		return errors.New("model is detached")
	}

	t := reflect.ValueOf(model).Elem()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.CanSet() {
			continue
		}

		typeField := t.Type().Field(i)
		keyName := typeField.Tag.Get("key")

		val, err := m.bucket.Get(utils.SKey(keyName))
		if err == nil {
			setFieldValue(field, val)
			m.loaded = true
		}
	}

	if defaultVal != nil {
		mergeModel(defaultVal, model)
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
			setFieldValue(field, fieldVal)
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
func setFieldValue(field reflect.Value, val any) {
	if !field.CanSet() || val == nil {
		return
	}

	v := reflect.ValueOf(val)
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			// Stop if we hit a nil pointer/interface
			return
		}
		v = v.Elem()
	}
	// Check if we ended up with a valid, concrete value.
	if !v.IsValid() {
		return
	}

	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		field.SetInt(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		field.SetUint(v.Uint())
	case reflect.Float32, reflect.Float64:
		field.SetFloat(v.Float())
	case reflect.String:
		field.SetString(v.String())
	case reflect.Bool:
		field.SetBool(v.Bool())
	default:
		panic("Can not set value of kind: " + field.Kind().String())
	}
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

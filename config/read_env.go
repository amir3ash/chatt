package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"
)

const TagName = "env"

var lookupEnv = os.LookupEnv

var primitiveParsers = map[reflect.Kind]func(fv reflect.Value, in string) error{
	reflect.String: func(fv reflect.Value, in string) error { fv.SetString(in); return nil },
	reflect.Bool:   setBool,
	reflect.Int:    func(fv reflect.Value, in string) error { return setInt(fv, in, 0) },
	reflect.Int8:   func(fv reflect.Value, in string) error { return setInt(fv, in, 8) },
	reflect.Int16:  func(fv reflect.Value, in string) error { return setInt(fv, in, 16) },
	reflect.Int32:  func(fv reflect.Value, in string) error { return setInt(fv, in, 32) },
	reflect.Int64:  func(fv reflect.Value, in string) error { return setInt(fv, in, 64) },

	reflect.Uint:   func(fv reflect.Value, in string) error { return setUInt(fv, in, 0) },
	reflect.Uint8:  func(fv reflect.Value, in string) error { return setUInt(fv, in, 8) },
	reflect.Uint16: func(fv reflect.Value, in string) error { return setUInt(fv, in, 16) },
	reflect.Uint32: func(fv reflect.Value, in string) error { return setUInt(fv, in, 32) },
	reflect.Uint64: func(fv reflect.Value, in string) error { return setUInt(fv, in, 64) },
}

func Parse[T any](conf *T) error {
	cType := reflect.TypeOf(conf).Elem()
	if cType.Kind() != reflect.Struct {
		return fmt.Errorf("conf type %v is not struct", cType)
	}

	cVal := reflect.ValueOf(conf).Elem()

	return parse(cType, cVal)
}

func parse(cType reflect.Type, cVal reflect.Value) error {
	if cType.Kind() != reflect.Struct {
		return fmt.Errorf("conf type %v is not struct", cType)
	}

	for i := 0; i < cType.NumField(); i++ {
		field := cType.Field(i)

		tag := field.Tag
		varName, hasEnv := tag.Lookup(TagName)
		if !hasEnv && !isStruct(field) {
			continue
		}

		required := tag.Get("required") == "true"
		envVariable, hasEnv := lookupEnv(varName)

		if defaultValue, hasDef := tag.Lookup("default"); !hasEnv && hasDef {
			envVariable = defaultValue
			hasEnv = true
		}

		if !isStruct(field) && (!hasEnv || envVariable == "") {
			if required {
				return fmt.Errorf("environment variable %s is required", varName)
			}
			continue
		}

		fieldVal := cVal.Field(i)

		if err := setField(fieldVal, field.Type.Kind(), envVariable); err != nil {
			return fmt.Errorf("can't parse env %s: %w", varName, err)
		}
	}

	return nil
}

func setField(fieldVal reflect.Value, fieldKind reflect.Kind, envVariable string) error {
	if !fieldVal.CanSet() {
		return nil
	}

	switch fieldVal.Interface().(type) {
	case time.Duration:
		v, err := time.ParseDuration(envVariable)
		if err != nil {
			return err
		}
		fieldVal.Set(reflect.ValueOf(v))
		return nil
	}

	if parseField, ok := primitiveParsers[fieldKind]; ok {
		return parseField(fieldVal, envVariable)
	}

	switch fieldKind {
	case reflect.Struct:
		return parse(fieldVal.Type(), fieldVal)

	case reflect.Ptr:
		fType := fieldVal.Type().Elem()

		newStruct := reflect.New(fType)
		fieldVal.Set(newStruct)

		return setField(fieldVal.Elem(), fieldVal.Elem().Kind(), envVariable)
	}

	return nil
}

func isStruct(sf reflect.StructField) bool {
	kind := sf.Type.Kind()
	return kind == reflect.Struct ||
		(kind == reflect.Ptr &&
			sf.Type.Elem().Kind() == reflect.Struct)
}

func setInt(fVal reflect.Value, input string, bitSize int) error {
	n, err := strconv.ParseInt(input, 10, bitSize)
	if err != nil {
		return err
	}

	fVal.SetInt(n)
	return nil
}

func setUInt(fVal reflect.Value, input string, bitSize int) error {
	n, err := strconv.ParseUint(input, 10, bitSize)
	if err != nil {
		return err
	}

	fVal.SetUint(n)
	return nil
}

func setBool(fVal reflect.Value, input string) error {
	b, err := strconv.ParseBool(input)
	if err != nil {
		return err
	}

	fVal.SetBool(b)
	return nil
}

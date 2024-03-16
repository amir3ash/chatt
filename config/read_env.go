package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
)

const TagName = "env"

var lookupEnv = os.LookupEnv

var parsers = map[reflect.Kind]func(fv reflect.Value, in string) error{
	reflect.Int:   func(fv reflect.Value, in string) error { return setInt(fv, in, 0) },
	reflect.Int8:  func(fv reflect.Value, in string) error { return setInt(fv, in, 8) },
	reflect.Int16: func(fv reflect.Value, in string) error { return setInt(fv, in, 16) },
	reflect.Int32: func(fv reflect.Value, in string) error { return setInt(fv, in, 32) },
	reflect.Int64: func(fv reflect.Value, in string) error { return setInt(fv, in, 64) },

	reflect.Uint:   func(fv reflect.Value, in string) error { return setUInt(fv, in, 0) },
	reflect.Uint8:  func(fv reflect.Value, in string) error { return setUInt(fv, in, 8) },
	reflect.Uint16: func(fv reflect.Value, in string) error { return setUInt(fv, in, 16) },
	reflect.Uint32: func(fv reflect.Value, in string) error { return setUInt(fv, in, 32) },
	reflect.Uint64: func(fv reflect.Value, in string) error { return setUInt(fv, in, 64) },
}

func parse[T any](conf *T) error {

	cType := reflect.TypeOf(conf).Elem()
	if cType.Kind() != reflect.Struct {
		return fmt.Errorf("conf type %v is not struct", cType)
	}

	cVal := reflect.ValueOf(conf).Elem()

	for i := 0; i < cType.NumField(); i++ {
		field := cType.Field(i)
		tag := field.Tag
		varName, ok := tag.Lookup(TagName)
		if !ok {
			continue
		}

		required := tag.Get("required") == "true"

		envVariable, ok := lookupEnv(varName)
		if !ok || envVariable == "" {
			if required {
				return fmt.Errorf("environment variable %s is required", varName)
			}
			continue
		}

		fieldVal := cVal.Field(i)
		if !fieldVal.CanSet() {
			continue
		}

		var err error

		switch field.Type.Kind() {
		case reflect.String:
			fieldVal.SetString(envVariable)

		case reflect.Int:
			err = setInt(fieldVal, envVariable, 0)

		case reflect.Int8:
			err = setInt(fieldVal, envVariable, 8)

		case reflect.Int16:
			err = setInt(fieldVal, envVariable, 16)

		case reflect.Int32:
			err = setInt(fieldVal, envVariable, 32)

		case reflect.Int64:
			err = setInt(fieldVal, envVariable, 64)

		case reflect.Uint:
			err = setUInt(fieldVal, envVariable, 0)

		case reflect.Uint8:
			err = setUInt(fieldVal, envVariable, 8)

		case reflect.Uint16:
			err = setUInt(fieldVal, envVariable, 16)

		case reflect.Uint32:
			err = setUInt(fieldVal, envVariable, 32)

		case reflect.Uint64:
			err = setUInt(fieldVal, envVariable, 64)

		case reflect.Bool:
			b, err := strconv.ParseBool(envVariable)
			if err != nil {
				return fmt.Errorf("can't parse env %s: %w", varName, err)
			}
			fieldVal.SetBool(b)
		}

		if err != nil {
			return fmt.Errorf("can't parse env %s: %w", varName, err)
		}
	}

	return nil
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

package config

import (
	"reflect"
	"testing"
	"time"
)

type mockEnvs map[string]string

func (m mockEnvs) LookupEnv(s string) (string, bool) {
	v, ok := m[s]
	return v, ok
}

func Test_Parse_empty(t *testing.T) {
	check := func(name string, err error, wantsErr bool) {
		t.Run(name, func(tt *testing.T) {
			if (err != nil) != wantsErr {
				t.Errorf("parse's err=%v, wantsErr=%v", err, wantsErr)
			}
			// t.Error(name, "  err=", err)
		})
	}

	trueVal := true
	var interfaceVal interface{} = struct{}{}
	check("must be struct", Parse(&trueVal), true)
	check("interface must be struct", Parse(&interfaceVal), true)
	check("must be not nil", Parse[time.Time](nil), true)
	check("struct without exported field", Parse(&time.Time{}), false)
	check("nil interface", Parse[interface{}](nil), true)
	check("simple empty struct", Parse(&struct{}{}), false)

	check("struct with unexported field", Parse(&struct{
		a *string `env:"NAME" required:"true"`
	}{}), false)
	
	check("struct with interface field", Parse(&struct{
		A interface{} `env:"NAME"`
	}{}), false)

	check("struct with *interface{} field", Parse(&struct{
		A *interface{} `env:"NAME"`
	}{}), false)

}


func Test_parse(t *testing.T) {
	testField(t, &struct {
		A time.Duration `env:"Dur"`
	}{}, mockEnvs{"Dur": "2ms"}, 2*time.Millisecond, false)

	testField(t, &struct {
		A *time.Duration `env:"Dur"`
	}{}, mockEnvs{"Dur": "2s"}, 2*time.Second, false)

	testField(t, &struct {
		A int `env:"AGE"`
	}{}, mockEnvs{"AGE": "23"}, 23, false)

	testField(t, &struct {
		A *int `env:"AGE"`
	}{}, mockEnvs{"AGE": "50"}, 50, false)

	testField(t, &struct {
		A int `env:"AGE" default:"23"`
	}{}, mockEnvs{}, 23, false)

	testField(t, &struct {
		A int `env:"AGE" required:"true"`
	}{}, mockEnvs{}, 0, true)

	testField(t, &struct {
		A int `env:"AGE" required:"true"`
	}{}, mockEnvs{"AGE": "0"}, 0, false)

	testField(t, &struct {
		A int `env:"AGE"`
	}{}, mockEnvs{"AGE": "23,"}, 0, true)

	testField(t, &struct {
		A int8 `env:"AGE"`
	}{}, mockEnvs{"AGE": "127"}, int8(127), false)

	testField(t, &struct {
		A int8 `env:"AGE"`
	}{}, mockEnvs{"AGE": "999"}, int8(0), true)

	testField(t, &struct {
		A uint8 `env:"AGE"`
	}{}, mockEnvs{"AGE": "23"}, uint8(23), false)

	testField(t, &struct {
		A int16 `env:"AGE"`
	}{}, mockEnvs{"AGE": "23"}, int16(23), false)

	testField(t, &struct {
		A int32 `env:"AGE"`
	}{}, mockEnvs{"AGE": "23"}, int32(23), false)

	testField(t, &struct {
		A int64 `env:"AGE"`
	}{}, mockEnvs{"AGE": "23"}, int64(23), false)

	testField(t, &struct {
		A string `env:"name"`
	}{}, mockEnvs{"name": "someString"}, "someString", false)

	testField(t, &struct {
		A string `env:"name" required:"true"`
	}{}, mockEnvs{"name": ""}, "", true) // must be not empty

	testField(t, &struct {
		A bool `env:"IS_TEST"`
	}{}, mockEnvs{"IS_TEST": "true"}, true, false)

	testField(t, &struct {
		A bool `env:"IS_TEST"`
	}{}, mockEnvs{"IS_TEST": "TRUE"}, true, false)

	testField(t, &struct {
		A bool `env:"IS_TEST" default:"true"`
	}{}, mockEnvs{}, true, false)

	testField(t, &struct {
		A bool `env:"AGE" required:"true"`
	}{}, mockEnvs{}, false, true)

	testField(t, &struct {
		A bool `env:"AGE" required:"true"`
	}{}, mockEnvs{"AGE": "false"}, false, false)

	testField(t, &struct {
		A bool `env:"IS_TEST"`
	}{}, mockEnvs{"IS_TEST": "1"}, true, false)

	testField(t, &struct {
		A bool `env:"IS_TEST"`
	}{}, mockEnvs{"NOT_EXISTS": ""}, false, false)

	testField(t, &struct {
		A bool `env:"IS_TEST"`
	}{}, mockEnvs{"IS_TEST": "NOT_EXPECTED"}, false, true)

	testField(t, &struct {
		A *bool `env:"IS_TEST"`
	}{}, mockEnvs{"IS_TEST": "true"}, true, false)

	testField(t, &struct {
		A *bool `env:"IS_TEST"`
	}{}, mockEnvs{"IS_TEST": "NOT_EXPECTED"}, false, true)

	testField(t, &struct {
		A uint8 `env:"IS_TEST"`
	}{}, mockEnvs{"NOT_EXISTS": ""}, uint8(0), false)

	testField(t, &struct {
		A uint16 `env:"AGE"`
	}{}, mockEnvs{"AGE": "23"}, uint16(23), false)

	testField(t, &struct {
		A uint32 `env:"IS_TEST"`
	}{}, mockEnvs{"IS_TEST": "NOT_EXPECTED"}, uint32(0), true)

	testField(t, &struct {
		A *uint64 `env:"IS_TEST"`
	}{}, mockEnvs{"IS_TEST": "4"}, uint64(4), false)

	testField(t, &struct {
		A *uint `env:"IS_TEST"`
	}{}, mockEnvs{"IS_TEST": "NOT_EXPECTED"}, uint(0), true)

}

func TestNestedConf(t *testing.T) {
	type nested struct {
		S int `env:"S"`
	}
	type testConf struct {
		N    int `env:"NUM"`
		Test nested
	}
	type testConfPointer struct {
		N    int `env:"NUM"`
		Test *nested
	}

	testStruct(t, &testConf{}, mockEnvs{"NUM": "2", "S": "5"}, &testConf{
		N:    2,
		Test: nested{S: 5},
	}, false)

	testStruct(t, &testConfPointer{}, mockEnvs{"NUM": "2", "S": "5"}, &testConfPointer{
		N:    2,
		Test: &nested{S: 5},
	}, false)
}

func testStruct[T any](t *testing.T, conf *T, envs mockEnvs, wants *T, wantsErr bool) {
	t.Helper()
	lookupEnv = envs.LookupEnv

	err := Parse(conf)
	if (err != nil) != wantsErr {
		t.Errorf("parse() error = %v, wantErr %v", err, wantsErr)
	}

	if !reflect.DeepEqual(conf, wants) {
		t.Errorf("can't parse config: struct=%++v, wants %++v", conf, wants)
	}
}

func testField[T any](t *testing.T, conf *T, envs mockEnvs, wants any, wantsErr bool) {
	t.Helper()
	lookupEnv = envs.LookupEnv

	err := Parse(conf)
	if (err != nil) != wantsErr {
		t.Errorf("parse() error = %v, wantErr %v", err, wantsErr)
	}

	elm := reflect.ValueOf(conf).Elem()

	// if elm.NumField() < 1 || !elm.Field(0).CanSet() {
	// 	return
	// }

	fValue := elm.Field(0)
	if fValue.Kind() == reflect.Ptr {
		fValue = fValue.Elem()
	}

	fVal := fValue.Interface()
	if fVal != wants {
		t.Errorf("can't parse config: field=%+v, wants %+v", fVal, wants)
	}
}

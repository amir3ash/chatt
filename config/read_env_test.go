package config

import (
	"reflect"
	"testing"
)

type mockEnvs map[string]string

func (m mockEnvs) LookupEnv(s string) (string, bool) {
	v, ok := m[s]
	return v, ok
}

func Test_parse(t *testing.T) {
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
		A string `env:"name"`
	}{}, mockEnvs{"name": "someString"}, "someString", false)

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

}

func TestNestedConf(t *testing.T) {
	type nested struct {
		S int `env:"S"`
	}
	type testConf struct {
		N    int `env:"NUM"`
		Test nested
	}
	type testConfPointer struct{
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

	fValue :=  elm.Field(0)
	if fValue.Kind() == reflect.Ptr {
		fValue = fValue.Elem()
	}

	fVal := fValue.Interface()
	if fVal != wants {
		t.Errorf("can't parse config: field=%+v, wants %+v", fVal, wants)
	}
}

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
	test(t, &struct {
		A int `env:"AGE"`
	}{}, mockEnvs{"AGE": "23"}, 23, false)

	test(t, &struct {
		A int `env:"AGE"`
	}{}, mockEnvs{"AGE": "23,"}, 0, true)

	test(t, &struct {
		A int8 `env:"AGE"`
	}{}, mockEnvs{"AGE": "127"}, int8(127), false)

	test(t, &struct {
		A int8 `env:"AGE"`
	}{}, mockEnvs{"AGE": "999"}, int8(0), true)

	test(t, &struct {
		A uint8 `env:"AGE"`
	}{}, mockEnvs{"AGE": "23"}, uint8(23), false)

	test(t, &struct {
		A int16 `env:"AGE"`
	}{}, mockEnvs{"AGE": "23"}, int16(23), false)

	test(t, &struct {
		A string `env:"name"`
	}{}, mockEnvs{"name": "someString"}, "someString", false)

	test(t, &struct {
		A bool `env:"IS_TEST"`
	}{}, mockEnvs{"IS_TEST": "true"}, true, false)

	test(t, &struct {
		A bool `env:"IS_TEST"`
	}{}, mockEnvs{"IS_TEST": "TRUE"}, true, false)

	test(t, &struct {
		A bool `env:"IS_TEST"`
	}{}, mockEnvs{"IS_TEST": "1"}, true, false)

	test(t, &struct {
		A bool `env:"IS_TEST"`
	}{}, mockEnvs{"NOT_EXISTS": ""}, false, false)

	test(t, &struct {
		A bool `env:"IS_TEST"`
	}{}, mockEnvs{"IS_TEST": "NOT_EXPECTED"}, false, true)

}


func test[T any](t *testing.T, conf *T, envs mockEnvs, wants any, wantsErr bool) {
	t.Helper()
	lookupEnv = envs.LookupEnv

	err := parse(conf)
	if (err != nil) != wantsErr {
		t.Errorf("parse() error = %v, wantErr %v", err, wantsErr)
	}

	elm := reflect.ValueOf(conf).Elem()
	
	// if elm.NumField() < 1 || !elm.Field(0).CanSet() {
	// 	return
	// }

	fVal := elm.Field(0).Interface()
	if fVal != wants {
		t.Errorf("can't parse config: field=%+v, wants %+v", fVal, wants)
	}
}

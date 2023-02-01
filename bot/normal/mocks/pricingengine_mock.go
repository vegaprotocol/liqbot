// Code generated by MockGen. DO NOT EDIT.
// Source: code.vegaprotocol.io/liqbot/bot/normal (interfaces: PricingEngine)

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	config "code.vegaprotocol.io/priceproxy/config"
	service "code.vegaprotocol.io/priceproxy/service"
	gomock "github.com/golang/mock/gomock"
)

// MockPricingEngine is a mock of PricingEngine interface.
type MockPricingEngine struct {
	ctrl     *gomock.Controller
	recorder *MockPricingEngineMockRecorder
}

// MockPricingEngineMockRecorder is the mock recorder for MockPricingEngine.
type MockPricingEngineMockRecorder struct {
	mock *MockPricingEngine
}

// NewMockPricingEngine creates a new mock instance.
func NewMockPricingEngine(ctrl *gomock.Controller) *MockPricingEngine {
	mock := &MockPricingEngine{ctrl: ctrl}
	mock.recorder = &MockPricingEngineMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPricingEngine) EXPECT() *MockPricingEngineMockRecorder {
	return m.recorder
}

// GetPrice mocks base method.
func (m *MockPricingEngine) GetPrice(arg0 config.PriceConfig) (service.PriceResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPrice", arg0)
	ret0, _ := ret[0].(service.PriceResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetPrice indicates an expected call of GetPrice.
func (mr *MockPricingEngineMockRecorder) GetPrice(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPrice", reflect.TypeOf((*MockPricingEngine)(nil).GetPrice), arg0)
}
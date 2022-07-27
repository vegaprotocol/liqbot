// Code generated by MockGen. DO NOT EDIT.
// Source: code.vegaprotocol.io/liqbot/data (interfaces: DataNode)

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	v1 "code.vegaprotocol.io/protos/data-node/api/v1"
	v10 "code.vegaprotocol.io/protos/vega/api/v1"
	gomock "github.com/golang/mock/gomock"
)

// MockDataNode is a mock of DataNode interface.
type MockDataNode struct {
	ctrl     *gomock.Controller
	recorder *MockDataNodeMockRecorder
}

// MockDataNodeMockRecorder is the mock recorder for MockDataNode.
type MockDataNodeMockRecorder struct {
	mock *MockDataNode
}

// NewMockDataNode creates a new mock instance.
func NewMockDataNode(ctrl *gomock.Controller) *MockDataNode {
	mock := &MockDataNode{ctrl: ctrl}
	mock.recorder = &MockDataNodeMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDataNode) EXPECT() *MockDataNodeMockRecorder {
	return m.recorder
}

// MarketDataByID mocks base method.
func (m *MockDataNode) MarketDataByID(arg0 *v1.MarketDataByIDRequest) (*v1.MarketDataByIDResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "MarketDataByID", arg0)
	ret0, _ := ret[0].(*v1.MarketDataByIDResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// MarketDataByID indicates an expected call of MarketDataByID.
func (mr *MockDataNodeMockRecorder) MarketDataByID(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MarketDataByID", reflect.TypeOf((*MockDataNode)(nil).MarketDataByID), arg0)
}

// ObserveEventBus mocks base method.
func (m *MockDataNode) ObserveEventBus() (v10.CoreService_ObserveEventBusClient, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ObserveEventBus")
	ret0, _ := ret[0].(v10.CoreService_ObserveEventBusClient)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ObserveEventBus indicates an expected call of ObserveEventBus.
func (mr *MockDataNodeMockRecorder) ObserveEventBus() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ObserveEventBus", reflect.TypeOf((*MockDataNode)(nil).ObserveEventBus))
}

// PartyAccounts mocks base method.
func (m *MockDataNode) PartyAccounts(arg0 *v1.PartyAccountsRequest) (*v1.PartyAccountsResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PartyAccounts", arg0)
	ret0, _ := ret[0].(*v1.PartyAccountsResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PartyAccounts indicates an expected call of PartyAccounts.
func (mr *MockDataNodeMockRecorder) PartyAccounts(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PartyAccounts", reflect.TypeOf((*MockDataNode)(nil).PartyAccounts), arg0)
}

// PositionsByParty mocks base method.
func (m *MockDataNode) PositionsByParty(arg0 *v1.PositionsByPartyRequest) (*v1.PositionsByPartyResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PositionsByParty", arg0)
	ret0, _ := ret[0].(*v1.PositionsByPartyResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PositionsByParty indicates an expected call of PositionsByParty.
func (mr *MockDataNodeMockRecorder) PositionsByParty(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PositionsByParty", reflect.TypeOf((*MockDataNode)(nil).PositionsByParty), arg0)
}

// PositionsSubscribe mocks base method.
func (m *MockDataNode) PositionsSubscribe(arg0 *v1.PositionsSubscribeRequest) (v1.TradingDataService_PositionsSubscribeClient, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PositionsSubscribe", arg0)
	ret0, _ := ret[0].(v1.TradingDataService_PositionsSubscribeClient)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PositionsSubscribe indicates an expected call of PositionsSubscribe.
func (mr *MockDataNodeMockRecorder) PositionsSubscribe(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PositionsSubscribe", reflect.TypeOf((*MockDataNode)(nil).PositionsSubscribe), arg0)
}
// Copyright 2021 Evmos Foundation
// This file is part of Evmos' Ethermint library.
//
// The Ethermint library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Ethermint library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Ethermint library. If not, see https://github.com/RWAs-labs/ethermint/blob/main/LICENSE
package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
)

const (
	TracerAccessList = "access_list"
	TracerJSON       = "json"
	TracerStruct     = "struct"
	TracerMarkdown   = "markdown"
)

// TxTraceResult is the result of a single transaction trace during a block trace.
type TxTraceResult struct {
	Result interface{} `json:"result,omitempty"` // Trace results produced by the tracer
	Error  string      `json:"error,omitempty"`  // Trace failure produced by the tracer
}

var _ vm.EVMLogger = &NoOpTracer{}

// NoOpTracer is an empty implementation of vm.Tracer interface
type NoOpTracer struct{}

// NewNoOpTracer creates a no-op vm.Tracer
func NewNoOpTracer() *NoOpTracer {
	return &NoOpTracer{}
}

// CaptureStart implements vm.Tracer interface
func (dt NoOpTracer) CaptureStart(_ *vm.EVM,
	_ common.Address,
	_ common.Address,
	_ bool,
	_ []byte,
	_ uint64,
	_ *big.Int) {
}

// CaptureState implements vm.Tracer interface
func (dt NoOpTracer) CaptureState(_ uint64, _ vm.OpCode, _, _ uint64, _ *vm.ScopeContext, _ []byte, _ int, _ error) {
}

// CaptureFault implements vm.Tracer interface
func (dt NoOpTracer) CaptureFault(_ uint64, _ vm.OpCode, _, _ uint64, _ *vm.ScopeContext, _ int, _ error) {
}

// CaptureEnd implements vm.Tracer interface
func (dt NoOpTracer) CaptureEnd(_ []byte, _ uint64, _ error) {}

// CaptureEnter implements vm.Tracer interface
func (dt NoOpTracer) CaptureEnter(_ vm.OpCode, _ common.Address, _ common.Address, _ []byte, _ uint64, _ *big.Int) {
}

// CaptureExit implements vm.Tracer interface
func (dt NoOpTracer) CaptureExit(_ []byte, _ uint64, _ error) {}

// CaptureTxStart implements vm.Tracer interface
func (dt NoOpTracer) CaptureTxStart(_ uint64) {}

// CaptureTxEnd implements vm.Tracer interface
func (dt NoOpTracer) CaptureTxEnd(_ uint64) {}

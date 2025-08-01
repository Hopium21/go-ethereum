// Copyright 2024 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package vm

import (
	gomath "math"

	"github.com/ethereum/go-ethereum/arbitrum/multigas"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

func gasSStore4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (*multigas.MultiGas, error) {
	gas := evm.AccessEvents.SlotGas(contract.Address(), stack.peek().Bytes32(), true)
	if gas == 0 {
		gas = params.WarmStorageReadCostEIP2929
	}
	return multigas.StorageAccessGas(gas), nil
}

func gasSLoad4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (*multigas.MultiGas, error) {
	gas := evm.AccessEvents.SlotGas(contract.Address(), stack.peek().Bytes32(), false)
	if gas == 0 {
		gas = params.WarmStorageReadCostEIP2929
	}
	// TODO(NIT-3484): Update multi dimensional gas here
	return multigas.UnknownGas(gas), nil
}

func gasBalance4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (*multigas.MultiGas, error) {
	if contract.IsSystemCall {
		return multigas.ZeroGas(), nil
	}
	address := stack.peek().Bytes20()
	gas := evm.AccessEvents.BasicDataGas(address, false)
	if gas == 0 {
		gas = params.WarmStorageReadCostEIP2929
	}
	// TODO(NIT-3484): Update multi dimensional gas here
	return multigas.UnknownGas(gas), nil
}

func gasExtCodeSize4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (*multigas.MultiGas, error) {
	address := stack.peek().Bytes20()
	if _, isPrecompile := evm.precompile(address); isPrecompile {
		return multigas.ZeroGas(), nil
	}
	if contract.IsSystemCall {
		return multigas.ZeroGas(), nil
	}
	gas := evm.AccessEvents.BasicDataGas(address, false)
	if gas == 0 {
		gas = params.WarmStorageReadCostEIP2929
	}
	// TODO(NIT-3484): Update multi dimensional gas here
	return multigas.UnknownGas(gas), nil
}

func gasExtCodeHash4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (*multigas.MultiGas, error) {
	if contract.IsSystemCall {
		return multigas.ZeroGas(), nil
	}
	address := stack.peek().Bytes20()
	if _, isPrecompile := evm.precompile(address); isPrecompile {
		return multigas.ZeroGas(), nil
	}
	gas := evm.AccessEvents.CodeHashGas(address, false)
	if gas == 0 {
		gas = params.WarmStorageReadCostEIP2929
	}
	// TODO(NIT-3484): Update multi dimensional gas here
	return multigas.UnknownGas(gas), nil
}

func makeCallVariantGasEIP4762(oldCalculator gasFunc) gasFunc {
	return func(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (*multigas.MultiGas, error) {
		multiGas, err := oldCalculator(evm, contract, stack, mem, memorySize)
		if err != nil {
			return multigas.ZeroGas(), err
		}
		if contract.IsSystemCall {
			return multiGas, nil
		}
		if _, isPrecompile := evm.precompile(contract.Address()); isPrecompile {
			return multiGas, nil
		}
		witnessGas := evm.AccessEvents.MessageCallGas(contract.Address())
		if witnessGas == 0 {
			witnessGas = params.WarmStorageReadCostEIP2929
		}
		// Witness gas considered as storage access.
		// See rationale in: https://github.com/OffchainLabs/nitro/blob/master/docs/decisions/0002-multi-dimensional-gas-metering.md
		multiGas.SafeIncrement(multigas.ResourceKindStorageAccess, witnessGas)
		return multiGas, nil
	}
}

var (
	gasCallEIP4762         = makeCallVariantGasEIP4762(gasCall)
	gasCallCodeEIP4762     = makeCallVariantGasEIP4762(gasCallCode)
	gasStaticCallEIP4762   = makeCallVariantGasEIP4762(gasStaticCall)
	gasDelegateCallEIP4762 = makeCallVariantGasEIP4762(gasDelegateCall)
)

func gasSelfdestructEIP4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (*multigas.MultiGas, error) {
	beneficiaryAddr := common.Address(stack.peek().Bytes20())
	if _, isPrecompile := evm.precompile(beneficiaryAddr); isPrecompile {
		return multigas.ZeroGas(), nil
	}
	if contract.IsSystemCall {
		return multigas.ZeroGas(), nil
	}
	contractAddr := contract.Address()
	statelessGas := evm.AccessEvents.BasicDataGas(contractAddr, false)
	if contractAddr != beneficiaryAddr {
		statelessGas += evm.AccessEvents.BasicDataGas(beneficiaryAddr, false)
	}
	// Charge write costs if it transfers value
	if evm.StateDB.GetBalance(contractAddr).Sign() != 0 {
		statelessGas += evm.AccessEvents.BasicDataGas(contractAddr, true)
		if contractAddr != beneficiaryAddr {
			statelessGas += evm.AccessEvents.BasicDataGas(beneficiaryAddr, true)
		}
	}
	// Value transfer considered as storage access.
	// See rationale in: https://github.com/OffchainLabs/nitro/blob/master/docs/decisions/0002-multi-dimensional-gas-metering.md
	return multigas.StorageAccessGas(statelessGas), nil
}

func gasCodeCopyEip4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (*multigas.MultiGas, error) {
	multiGas, err := gasCodeCopy(evm, contract, stack, mem, memorySize)
	if err != nil {
		return multigas.ZeroGas(), err
	}
	var (
		codeOffset = stack.Back(1)
		length     = stack.Back(2)
	)
	uint64CodeOffset, overflow := codeOffset.Uint64WithOverflow()
	if overflow {
		uint64CodeOffset = gomath.MaxUint64
	}
	_, copyOffset, nonPaddedCopyLength := getDataAndAdjustedBounds(contract.Code, uint64CodeOffset, length.Uint64())

	// TODO(NIT-3484): Update multi dimensional gas here
	if !contract.IsDeployment && !contract.IsSystemCall {
		gas := evm.AccessEvents.CodeChunksRangeGas(contract.Address(), copyOffset, nonPaddedCopyLength, uint64(len(contract.Code)), false)
		multiGas.SafeIncrement(multigas.ResourceKindUnknown, gas)
	}
	return multiGas, nil
}

func gasExtCodeCopyEIP4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (*multigas.MultiGas, error) {
	// memory expansion first (dynamic part of pre-2929 implementation)
	multiGas, err := gasExtCodeCopy(evm, contract, stack, mem, memorySize)
	if err != nil {
		return multigas.ZeroGas(), err
	}
	if contract.IsSystemCall {
		return multiGas, nil
	}
	addr := common.Address(stack.peek().Bytes20())
	wgas := evm.AccessEvents.BasicDataGas(addr, false)
	if wgas == 0 {
		wgas = params.WarmStorageReadCostEIP2929
	}
	// We charge (cold-warm), since 'warm' is already charged as constantGas
	// TODO(NIT-3484): Update multi dimensional gas here
	if overflow := multiGas.SafeIncrement(multigas.ResourceKindUnknown, wgas); overflow {
		return multigas.ZeroGas(), ErrGasUintOverflow
	}
	return multiGas, nil
}

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
package network

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"cosmossdk.io/log"
	cmtcfg "github.com/cometbft/cometbft/config"
	tmos "github.com/cometbft/cometbft/libs/os"
	"github.com/cometbft/cometbft/node"
	"github.com/cometbft/cometbft/p2p"
	pvm "github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	"github.com/cometbft/cometbft/rpc/client/local"
	"github.com/cometbft/cometbft/types"
	cmttime "github.com/cometbft/cometbft/types/time"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/sync/errgroup"

	sdkserver "github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	servergrpc "github.com/cosmos/cosmos-sdk/server/grpc"
	servercmtlog "github.com/cosmos/cosmos-sdk/server/log"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	mintypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/RWAs-labs/ethermint/server"
	evmtypes "github.com/RWAs-labs/ethermint/x/evm/types"
)

func startInProcess(cfg Config, val *Validator) error {
	logger := val.Ctx.Logger
	cmtCfg := val.Ctx.Config
	cmtCfg.Instrumentation.Prometheus = false

	if err := val.AppConfig.ValidateBasic(); err != nil {
		return err
	}

	nodeKey, err := p2p.LoadOrGenNodeKey(cmtCfg.NodeKeyFile())
	if err != nil {
		return err
	}

	app := cfg.AppConstructor(*val)

	genDocProvider := node.DefaultGenesisDocProviderFunc(cmtCfg)
	cmtApp := sdkserver.NewCometABCIWrapper(app)
	tmNode, err := node.NewNode(
		cmtCfg,
		pvm.LoadOrGenFilePV(cmtCfg.PrivValidatorKeyFile(), cmtCfg.PrivValidatorStateFile()),
		nodeKey,
		proxy.NewLocalClientCreator(cmtApp),
		genDocProvider,
		cmtcfg.DefaultDBProvider,
		node.DefaultMetricsProvider(cmtCfg.Instrumentation),
		servercmtlog.CometLoggerWrapper{Logger: logger.With("module", val.Moniker)},
	)
	if err != nil {
		return err
	}

	if err := tmNode.Start(); err != nil {
		return err
	}

	val.tmNode = tmNode

	if val.RPCAddress != "" {
		val.RPCClient = local.New(tmNode)
	}

	// We'll need a RPC client if the validator exposes a gRPC or REST endpoint.
	if val.APIAddress != "" || val.AppConfig.GRPC.Enable {
		val.ClientCtx = val.ClientCtx.
			WithClient(val.RPCClient)

		// Add the tx service in the gRPC router.
		app.RegisterTxService(val.ClientCtx)

		// Add the tendermint queries service in the gRPC router.
		app.RegisterTendermintService(val.ClientCtx)
		app.RegisterNodeService(val.ClientCtx, val.AppConfig.Config)
	}

	ctx := context.Background()
	ctx, val.cancelFn = context.WithCancel(ctx)
	val.errGroup, ctx = errgroup.WithContext(ctx)

	if val.AppConfig.API.Enable && val.APIAddress != "" {
		apiSrv := api.New(val.ClientCtx, logger.With("module", "api-server"), val.grpc)
		app.RegisterAPIRoutes(apiSrv, val.AppConfig.API)

		val.errGroup.Go(func() error {
			return apiSrv.Start(ctx, val.AppConfig.Config)
		})

		val.api = apiSrv
	}

	if val.AppConfig.GRPC.Enable {
		grpcSrv, err := servergrpc.NewGRPCServer(val.ClientCtx, app, val.AppConfig.GRPC)
		if err != nil {
			return err
		}

		// Start the gRPC server in a goroutine. Note, the provided ctx will ensure
		// that the server is gracefully shut down.
		val.errGroup.Go(func() error {
			return servergrpc.StartGRPCServer(ctx, logger.With(log.ModuleKey, "grpc-server"), val.AppConfig.GRPC, grpcSrv)
		})

		val.grpc = grpcSrv
	}

	if val.AppConfig.JSONRPC.Enable && val.AppConfig.JSONRPC.Address != "" {
		if val.Ctx == nil || val.Ctx.Viper == nil {
			return fmt.Errorf("validator %s context is nil", val.Moniker)
		}

		tmEndpoint := "/websocket"
		tmRPCAddr := val.RPCAddress
		val.jsonrpc, val.jsonrpcDone, err = server.StartJSONRPC(val.Ctx, val.ClientCtx, tmRPCAddr, tmEndpoint, val.AppConfig, nil)
		if err != nil {
			return err
		}

		address := fmt.Sprintf("http://%s", val.AppConfig.JSONRPC.Address)

		val.JSONRPCClient, err = ethclient.Dial(address)
		if err != nil {
			return fmt.Errorf("failed to dial JSON-RPC at %s: %w", val.AppConfig.JSONRPC.Address, err)
		}
	}

	return nil
}

func collectGenFiles(cfg Config, vals []*Validator, outputDir string) error {
	genTime := cmttime.Now()

	for i := 0; i < cfg.NumValidators; i++ {
		tmCfg := vals[i].Ctx.Config

		nodeDir := filepath.Join(outputDir, vals[i].Moniker, "evmosd")
		gentxsDir := filepath.Join(outputDir, "gentxs")

		tmCfg.Moniker = vals[i].Moniker
		tmCfg.SetRoot(nodeDir)

		initCfg := genutiltypes.NewInitConfig(cfg.ChainID, gentxsDir, vals[i].NodeID, vals[i].PubKey)

		genFile := tmCfg.GenesisFile()
		genDoc, err := genutiltypes.AppGenesisFromFile(genFile)
		if err != nil {
			return err
		}

		appState, err := genutil.GenAppStateFromConfig(cfg.Codec, cfg.TxConfig,
			tmCfg,
			initCfg,
			genDoc,
			banktypes.GenesisBalancesIterator{},
			genutiltypes.DefaultMessageValidator,
			cfg.TxConfig.SigningContext().ValidatorAddressCodec(),
		)
		if err != nil {
			return err
		}

		// overwrite each validator's genesis file to have a canonical genesis time
		if err := genutil.ExportGenesisFileWithTime(genFile, cfg.ChainID, nil, appState, genTime); err != nil {
			return err
		}
	}

	return nil
}

func initGenFiles(cfg Config, genAccounts []authtypes.GenesisAccount, genBalances []banktypes.Balance, genFiles []string) error {
	// set the accounts in the genesis state
	var authGenState authtypes.GenesisState
	cfg.Codec.MustUnmarshalJSON(cfg.GenesisState[authtypes.ModuleName], &authGenState)

	accounts, err := authtypes.PackAccounts(genAccounts)
	if err != nil {
		return err
	}

	authGenState.Accounts = append(authGenState.Accounts, accounts...)
	cfg.GenesisState[authtypes.ModuleName] = cfg.Codec.MustMarshalJSON(&authGenState)

	// set the balances in the genesis state
	var bankGenState banktypes.GenesisState
	cfg.Codec.MustUnmarshalJSON(cfg.GenesisState[banktypes.ModuleName], &bankGenState)

	bankGenState.Balances = genBalances
	cfg.GenesisState[banktypes.ModuleName] = cfg.Codec.MustMarshalJSON(&bankGenState)

	var stakingGenState stakingtypes.GenesisState
	cfg.Codec.MustUnmarshalJSON(cfg.GenesisState[stakingtypes.ModuleName], &stakingGenState)

	stakingGenState.Params.BondDenom = cfg.BondDenom
	cfg.GenesisState[stakingtypes.ModuleName] = cfg.Codec.MustMarshalJSON(&stakingGenState)

	var govGenState govv1.GenesisState
	cfg.Codec.MustUnmarshalJSON(cfg.GenesisState[govtypes.ModuleName], &govGenState)

	govGenState.Params.MinDeposit[0].Denom = cfg.BondDenom
	cfg.GenesisState[govtypes.ModuleName] = cfg.Codec.MustMarshalJSON(&govGenState)

	var mintGenState mintypes.GenesisState
	cfg.Codec.MustUnmarshalJSON(cfg.GenesisState[mintypes.ModuleName], &mintGenState)

	mintGenState.Params.MintDenom = cfg.BondDenom
	cfg.GenesisState[mintypes.ModuleName] = cfg.Codec.MustMarshalJSON(&mintGenState)

	var crisisGenState crisistypes.GenesisState
	cfg.Codec.MustUnmarshalJSON(cfg.GenesisState[crisistypes.ModuleName], &crisisGenState)

	crisisGenState.ConstantFee.Denom = cfg.BondDenom
	cfg.GenesisState[crisistypes.ModuleName] = cfg.Codec.MustMarshalJSON(&crisisGenState)

	var evmGenState evmtypes.GenesisState
	cfg.Codec.MustUnmarshalJSON(cfg.GenesisState[evmtypes.ModuleName], &evmGenState)

	evmGenState.Params.EvmDenom = cfg.BondDenom
	cfg.GenesisState[evmtypes.ModuleName] = cfg.Codec.MustMarshalJSON(&evmGenState)

	appGenStateJSON, err := json.MarshalIndent(cfg.GenesisState, "", "  ")
	if err != nil {
		return err
	}

	genDoc := types.GenesisDoc{
		ChainID:    cfg.ChainID,
		AppState:   appGenStateJSON,
		Validators: nil,
	}

	// generate empty genesis files for each validator and save
	for i := 0; i < cfg.NumValidators; i++ {
		if err := genDoc.SaveAs(genFiles[i]); err != nil {
			return err
		}
	}

	return nil
}

func WriteFile(name string, dir string, contents []byte) error {
	file := filepath.Join(dir, name)

	err := tmos.EnsureDir(dir, 0o755)
	if err != nil {
		return err
	}

	return tmos.WriteFile(file, contents, 0o644)
}

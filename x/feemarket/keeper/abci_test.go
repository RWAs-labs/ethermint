package keeper_test

import (
	"fmt"

	storetypes "cosmossdk.io/store/types"
)

func (suite *KeeperTestSuite) TestEndBlock() {
	testCases := []struct {
		name         string
		NoBaseFee    bool
		malleate     func()
		expGasWanted uint64
	}{
		{
			"baseFee nil",
			true,
			func() {},
			uint64(0),
		},
		{
			"pass",
			false,
			func() {
				meter := storetypes.NewGasMeter(uint64(1000000000))
				suite.ctx = suite.ctx.WithBlockGasMeter(meter)
				suite.app.FeeMarketKeeper.SetTransientBlockGasWanted(suite.ctx, 5000000)
			},
			uint64(2500000),
		},
	}
	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.SetupTest() // reset
			params := suite.app.FeeMarketKeeper.GetParams(suite.ctx)
			params.NoBaseFee = tc.NoBaseFee
			suite.app.FeeMarketKeeper.SetParams(suite.ctx, params)

			tc.malleate()

			suite.app.FeeMarketKeeper.EndBlock(suite.ctx)
			gasWanted := suite.app.FeeMarketKeeper.GetBlockGasWanted(suite.ctx)
			suite.Require().Equal(tc.expGasWanted, gasWanted, tc.name)
		})
	}
}

package keeper_test

import (
	"fmt"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"

	abcitypes "github.com/tendermint/tendermint/abci/types"

	"github.com/cosmos/ibc-apps/modules/async-icq/v4/testing/simapp"
	"github.com/cosmos/ibc-apps/modules/async-icq/v4/types"
	clienttypes "github.com/cosmos/ibc-go/v4/modules/core/02-client/types"
	channeltypes "github.com/cosmos/ibc-go/v4/modules/core/04-channel/types"
	ibctesting "github.com/cosmos/ibc-go/v4/testing"
)

func (suite *KeeperTestSuite) TestOnRecvPacket() {
	var (
		path       *ibctesting.Path
		packetData []byte
	)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"icq successfully queries banktypes.AllBalances",
			func() {
				q := banktypes.QueryAllBalancesRequest{
					Address: suite.chainA.SenderAccount.GetAddress().String(),
					Pagination: &query.PageRequest{
						Offset: 0,
						Limit:  10,
					},
				}
				reqs := []abcitypes.RequestQuery{
					{
						Path: "/cosmos.bank.v1beta1.Query/AllBalances",
						Data: simapp.GetSimApp(suite.chainA).AppCodec().MustMarshal(&q),
					},
				}
				data, err := types.SerializeCosmosQuery(reqs)
				suite.Require().NoError(err)

				icqPacketData := types.InterchainQueryPacketData{
					Data: data,
				}
				packetData = icqPacketData.GetBytes()

				params := types.NewParams(true, []string{"/cosmos.bank.v1beta1.Query/AllBalances"})
				simapp.GetSimApp(suite.chainB).ICQKeeper.SetParams(suite.chainB.GetContext(), params)
			},
			true,
		},
		{
			"cannot unmarshal interchain query packet data",
			func() {
				packetData = []byte{}
			},
			false,
		},
		{
			"cannot deserialize interchain query packet data messages",
			func() {
				data := []byte("invalid packet data")

				icaPacketData := types.InterchainQueryPacketData{
					Data: data,
				}

				packetData = icaPacketData.GetBytes()
			},
			false,
		},
		{
			"unauthorised: message type not allowed", // NOTE: do not update params to explicitly force the error
			func() {
				q := banktypes.QueryAllBalancesRequest{}
				reqs := []abcitypes.RequestQuery{
					{
						Path: "/cosmos.bank.v1beta1.Query/AllBalances",
						Data: simapp.GetSimApp(suite.chainA).AppCodec().MustMarshal(&q),
					},
				}
				data, err := types.SerializeCosmosQuery(reqs)
				suite.Require().NoError(err)

				icaPacketData := types.InterchainQueryPacketData{
					Data: data,
				}
				packetData = icaPacketData.GetBytes()
			},
			false,
		},
		{
			"unauthorised: can not perform historical query (i.e. height != 0)",
			func() {
				q := banktypes.QueryAllBalancesRequest{}
				reqs := []abcitypes.RequestQuery{
					{
						Path:   "/cosmos.bank.v1beta1.Query/AllBalances",
						Data:   simapp.GetSimApp(suite.chainA).AppCodec().MustMarshal(&q),
						Height: 1,
					},
				}
				data, err := types.SerializeCosmosQuery(reqs)
				suite.Require().NoError(err)

				icaPacketData := types.InterchainQueryPacketData{
					Data: data,
				}
				packetData = icaPacketData.GetBytes()

				params := types.NewParams(true, []string{"/cosmos.bank.v1beta1.Query/AllBalances"})
				simapp.GetSimApp(suite.chainB).ICQKeeper.SetParams(suite.chainB.GetContext(), params)
			},
			false,
		},
		{
			"unauthorised: can not fetch query proof (i.e. prove == true)",
			func() {
				q := banktypes.QueryAllBalancesRequest{}
				reqs := []abcitypes.RequestQuery{
					{
						Path:  "/cosmos.bank.v1beta1.Query/AllBalances",
						Data:  simapp.GetSimApp(suite.chainA).AppCodec().MustMarshal(&q),
						Prove: true,
					},
				}
				data, err := types.SerializeCosmosQuery(reqs)
				suite.Require().NoError(err)

				icaPacketData := types.InterchainQueryPacketData{
					Data: data,
				}
				packetData = icaPacketData.GetBytes()

				params := types.NewParams(true, []string{"/cosmos.bank.v1beta1.Query/AllBalances"})
				simapp.GetSimApp(suite.chainB).ICQKeeper.SetParams(suite.chainB.GetContext(), params)
			},
			false,
		},
	}

	for _, tc := range testCases {
		tc := tc

		suite.Run(tc.msg, func() {
			suite.SetupTest() // reset

			path = NewICQPath(suite.chainA, suite.chainB)
			suite.coordinator.SetupConnections(path)

			err := SetupICQPath(path)
			suite.Require().NoError(err)

			tc.malleate() // malleate mutates test data

			packet := channeltypes.NewPacket(
				packetData,
				suite.chainA.SenderAccount.GetSequence(),
				path.EndpointA.ChannelConfig.PortID,
				path.EndpointA.ChannelID,
				path.EndpointB.ChannelConfig.PortID,
				path.EndpointB.ChannelID,
				clienttypes.NewHeight(1, 100),
				0,
			)

			txResponse, err := simapp.GetSimApp(suite.chainB).ICQKeeper.OnRecvPacket(suite.chainB.GetContext(), packet)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().NotNil(txResponse)
			} else {
				suite.Require().Error(err)
				suite.Require().Nil(txResponse)
			}
		})
	}
}

func (suite *KeeperTestSuite) TestOutOfGasOnSlowQueries() {
	suite.SetupTest() // reset

	path := NewICQPath(suite.chainA, suite.chainB)
	suite.coordinator.SetupConnections(path)

	err := SetupICQPath(path)
	suite.Require().NoError(err)

	q := banktypes.QueryAllBalancesRequest{
		Address: suite.chainA.SenderAccount.GetAddress().String(),
		Pagination: &query.PageRequest{
			Offset: 0,
			Limit:  1000000,
		},
	}
	reqs := []abcitypes.RequestQuery{
		{
			Path: "/cosmos.bank.v1beta1.Query/AllBalances",
			Data: simapp.GetSimApp(suite.chainA).AppCodec().MustMarshal(&q),
		},
	}
	data, err := types.SerializeCosmosQuery(reqs)
	suite.Require().NoError(err)

	icqPacketData := types.InterchainQueryPacketData{
		Data: data,
	}
	packetData := icqPacketData.GetBytes()

	params := types.NewParams(true, []string{"/cosmos.bank.v1beta1.Query/AllBalances"})
	simapp.GetSimApp(suite.chainB).ICQKeeper.SetParams(suite.chainB.GetContext(), params)

	packet := channeltypes.NewPacket(
		packetData,
		suite.chainA.SenderAccount.GetSequence(),
		path.EndpointA.ChannelConfig.PortID,
		path.EndpointA.ChannelID,
		path.EndpointB.ChannelConfig.PortID,
		path.EndpointB.ChannelID,
		clienttypes.NewHeight(1, 100),
		0,
	)

	ctx := suite.chainB.GetContext()
	ctx = ctx.WithGasMeter(sdk.NewGasMeter(2000))

	// enough gas for this small query, but not for the larger one. This one should work
	_, err = simapp.GetSimApp(suite.chainB).ICQKeeper.OnRecvPacket(ctx, packet)
	suite.Require().NoError(err)

	// fund account with 100_000 denoms
	for i := 0; i < 150_000; i++ {
		denom := fmt.Sprintf("denom%d", i)
		simapp.GetSimApp(suite.chainA).BankKeeper.MintCoins(suite.chainA.GetContext(), minttypes.ModuleName, sdk.NewCoins(sdk.NewInt64Coin(denom, 10)))
		simapp.GetSimApp(suite.chainA).BankKeeper.SendCoinsFromModuleToAccount(suite.chainA.GetContext(), types.ModuleName, suite.chainA.SenderAccount.GetAddress(), sdk.NewCoins(sdk.NewInt64Coin(denom, 10)))
	}

	packet = channeltypes.NewPacket(
		packetData,
		suite.chainA.SenderAccount.GetSequence(),
		path.EndpointA.ChannelConfig.PortID,
		path.EndpointA.ChannelID,
		path.EndpointB.ChannelConfig.PortID,
		path.EndpointB.ChannelID,
		clienttypes.NewHeight(1, 100),
		0,
	)

	// and this one should panic with 'out of gas
	suite.Assert().Panics(func() {
		simapp.GetSimApp(suite.chainB).ICQKeeper.OnRecvPacket(ctx, packet)
	}, "out of gas")

}

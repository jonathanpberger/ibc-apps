package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-apps/modules/async-icq/v7/types"
)

func TestValidateParams(t *testing.T) {
	require.NoError(t, types.DefaultParams().Validate())
	require.NoError(t, types.NewParams(false, []string{}).Validate())
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package modify_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Chaguar/common/tools/fxadmin/core/modify"
)

// TestHandlerNotImplemented asserts every modify command currently returns
// ErrNotImplemented, since the change logic is not implemented yet.
func TestHandlerNotImplemented(t *testing.T) {
	t.Parallel()

	h := modify.New()

	require.ErrorIs(t, h.AppRemove("peer2", "next.pb"), modify.ErrNotImplemented)
	require.ErrorIs(t, h.AppKnownCertsAdd("peer1", []string{"c.pem"}, "next.pb"), modify.ErrNotImplemented)
	require.ErrorIs(t, h.AppKnownCertsRemove("peer1", []string{"c.pem"}, "next.pb"), modify.ErrNotImplemented)
}

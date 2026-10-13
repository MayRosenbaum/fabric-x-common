/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package modify_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/Chaguar/common/common/channelconfig"
	"github.com/Chaguar/common/protolator/protoext/peerext"
	"github.com/Chaguar/common/tools/cryptogen"
	"github.com/Chaguar/common/tools/fxadmin/core/modify"
	"github.com/Chaguar/common/utils/test"
)

const (
	// appTestNewOrg is the name and MSP ID of the application org the tests
	// add, and appTestExistingOrg the application org of the block they edit.
	// appTestOtherName is a name that no org of the block has.
	appTestNewOrg      = "peer2"
	appTestExistingOrg = "peer-org-1"
	appTestOtherName   = "peer3"
	// The peers of each application org the tests generate.
	appTestPeer0 = "peer0"
	appTestPeer1 = "peer1"
	// The formats an organization definition is written in.
	appTestFormatYAML = "yaml"
	appTestFormatJSON = "json"
	// The keys of an organization definition.
	appTestNameKey        = "name"
	appTestConfigGroupKey = "config_group"
)

// TestAppAdd adds an application org from its definition, the org's name and
// its config group in the form it has in the config block, and checks the
// edited block: the org's group is exactly the group of the definition, nothing
// else in the config changes, and the block still loads as a channel config.
func TestAppAdd(t *testing.T) {
	t.Parallel()

	baseBlock := newAppTestBlock(t)
	org := newAppTestOrgGroup(t)

	for _, tc := range []struct {
		name   string
		format string
	}{
		{name: "YAML definition", format: appTestFormatYAML},
		{name: "JSON definition, as fxadmin decode prints the group", format: appTestFormatJSON},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			blockPath := writeNodeTestBlock(t, baseBlock)
			orgPath := writeAppTestDefinition(t, appTestDefinition(t, appTestNewOrg, org, tc.format))
			require.NoError(t, modify.New().AppAdd(orgPath, blockPath))

			// The org is added exactly as defined.
			editedBlock := readNodeTestFile(t, blockPath)
			edited := nodeTestConfigOf(t, editedBlock)
			application := edited.GetChannelGroup().GetGroups()[channelconfig.ApplicationGroupKey]
			test.RequireProtoEqual(t, org, application.GetGroups()[appTestNewOrg])

			// Apart from the new org, the whole config is unchanged.
			delete(application.Groups, appTestNewOrg)
			test.RequireProtoEqual(t, nodeTestConfigOf(t, baseBlock), edited)

			// The edited block loads as a channel config that includes the new org.
			material, err := channelconfig.LoadConfigBlockMaterialFromFile(blockPath)
			require.NoError(t, err)
			mspIDs := make([]string, 0, len(material.ApplicationOrganizations))
			for _, appOrg := range material.ApplicationOrganizations {
				mspIDs = append(mspIDs, appOrg.MspID)
			}
			require.ElementsMatch(t, []string{appTestExistingOrg, appTestNewOrg}, mspIDs)

			// The block was written in place.
			info, err := os.Stat(blockPath)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(nodeTestBlockMode), info.Mode().Perm())
		})
	}
}

// TestAppAddREADMEDefinition adds an application org from a definition written
// as the fxadmin README shows it: comments, null fields, empty lists, and the
// unquoted n key of the policy rules, with the certificates of the org given
// as base64-encoded PEM. The org's MSP then holds exactly those certificates.
func TestAppAddREADMEDefinition(t *testing.T) {
	t.Parallel()

	msp := nodeTestFabricMSPConfig(t, newAppTestOrgGroup(t))
	require.NotEmpty(t, msp.GetAdmins())
	require.GreaterOrEqual(t, len(msp.GetKnownCerts()), 2)
	known := msp.GetKnownCerts()[:2]
	definition := fmt.Sprintf(appTestREADMEDefinition,
		base64.StdEncoding.EncodeToString(msp.GetRootCerts()[0]),
		base64.StdEncoding.EncodeToString(msp.GetAdmins()[0]),
		base64.StdEncoding.EncodeToString(msp.GetTlsRootCerts()[0]),
		base64.StdEncoding.EncodeToString(known[0]),
		base64.StdEncoding.EncodeToString(known[1]))

	blockPath := writeNodeTestBlock(t, newAppTestBlock(t))
	require.NoError(t, modify.New().AppAdd(writeAppTestDefinition(t, definition), blockPath))

	application := nodeTestConfigOf(t, readNodeTestFile(t, blockPath)).GetChannelGroup().
		GetGroups()[channelconfig.ApplicationGroupKey]
	added := application.GetGroups()[appTestNewOrg]
	require.NotNil(t, added)
	require.Len(t, added.GetPolicies(), 4)
	addedMSP := nodeTestFabricMSPConfig(t, added)
	require.Equal(t, appTestNewOrg, addedMSP.GetName())
	require.Equal(t, [][]byte{msp.GetRootCerts()[0]}, addedMSP.GetRootCerts())
	require.Equal(t, [][]byte{msp.GetAdmins()[0]}, addedMSP.GetAdmins())
	require.Equal(t, [][]byte{msp.GetTlsRootCerts()[0]}, addedMSP.GetTlsRootCerts())
	require.Equal(t, known, addedMSP.GetKnownCerts())
	require.Nil(t, addedMSP.GetFabricNodeOus())
}

// TestAppAddCopyOfAnOrg adds an application org of the block again, under
// another name. The channel config accepts two orgs with the same MSP ID when
// their MSPs are identical, and so does the command.
func TestAppAddCopyOfAnOrg(t *testing.T) {
	t.Parallel()

	baseBlock := newAppTestBlock(t)
	existing := nodeTestConfigOf(t, baseBlock).GetChannelGroup().GetGroups()[channelconfig.ApplicationGroupKey].
		GetGroups()[appTestExistingOrg]
	require.NotNil(t, existing)

	blockPath := writeNodeTestBlock(t, baseBlock)
	orgPath := writeAppTestDefinition(t, appTestDefinition(t, appTestOtherName, existing, appTestFormatYAML))
	require.NoError(t, modify.New().AppAdd(orgPath, blockPath))

	application := nodeTestConfigOf(t, readNodeTestFile(t, blockPath)).GetChannelGroup().
		GetGroups()[channelconfig.ApplicationGroupKey]
	require.Len(t, application.GetGroups(), 2)
	test.RequireProtoEqual(t, existing, application.GetGroups()[appTestExistingOrg])
	test.RequireProtoEqual(t, existing, application.GetGroups()[appTestOtherName])
}

// TestAppAddRejects covers definitions and conflicts that must fail, and
// asserts that a failed add leaves the block file unchanged.
func TestAppAddRejects(t *testing.T) {
	t.Parallel()

	baseBlock := newAppTestBlock(t)
	org := newAppTestOrgGroup(t)
	orgMap := protolatorMapOf(t, &peerext.DynamicApplicationOrgGroup{ConfigGroup: org})
	withoutMSP := proto.CloneOf(org)
	delete(withoutMSP.Values, channelconfig.MSPKey)

	for _, tc := range []struct {
		name       string
		block      []byte
		definition string
		wantErr    string
	}{
		{
			name: "an org name already in the Application group", block: baseBlock,
			definition: appTestDefinition(t, appTestExistingOrg, org, appTestFormatYAML),
			wantErr:    `application org "peer-org-1" already exists`,
		},
		{
			// Two orgs may share an MSP ID only when their MSPs are identical; see
			// TestAppAddCopyOfAnOrg.
			name: "an org with the MSP ID of an application org but another MSP", block: baseBlock,
			definition: appTestDefinition(t, appTestNewOrg, withAppTestMSPName(t, org, appTestExistingOrg),
				appTestFormatYAML),
			wantErr: "Attempted to define two different versions of MSP",
		},
		{
			name: "no name", block: baseBlock,
			definition: marshalDefinition(t, map[string]any{appTestConfigGroupKey: orgMap}),
			wantErr:    "has no name",
		},
		{
			name: "a name that is not a string", block: baseBlock,
			definition: marshalDefinition(t, map[string]any{appTestNameKey: 5, appTestConfigGroupKey: orgMap}),
			wantErr:    "has no name",
		},
		{
			name: "no config group", block: baseBlock,
			definition: marshalDefinition(t, map[string]any{appTestNameKey: appTestNewOrg}),
			wantErr:    "has no config_group",
		},
		{
			// The old form of a definition: a Name key next to the group's own keys.
			name: "an unknown key", block: baseBlock,
			definition: marshalDefinition(t, map[string]any{"Name": appTestNewOrg, appTestConfigGroupKey: orgMap}),
			wantErr:    `has an unknown key "Name": want "name" and "config_group"`,
		},
		{
			name: "not YAML", block: baseBlock,
			definition: "name: [peer2",
			wantErr:    "failed to parse organization definition",
		},
		{
			name: "a config group key that is not a field of a config group", block: baseBlock,
			definition: marshalDefinition(t, map[string]any{
				appTestNameKey: appTestNewOrg, appTestConfigGroupKey: map[string]any{"not_a_field": 1},
			}),
			wantErr: "failed to decode the config_group of organization definition",
		},
		{
			name: "an org group without an MSP", block: baseBlock,
			definition: appTestDefinition(t, appTestNewOrg, withoutMSP, appTestFormatYAML),
			wantErr:    "the edited config is not a valid channel config",
		},
		{
			name: "a block file that is not a config block", block: []byte("not a block"),
			definition: appTestDefinition(t, appTestNewOrg, org, appTestFormatYAML),
			wantErr:    "unmarshal config block",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			blockPath := writeNodeTestBlock(t, tc.block)
			orgPath := writeAppTestDefinition(t, tc.definition)
			err := modify.New().AppAdd(orgPath, blockPath)
			require.ErrorContains(t, err, tc.wantErr)
			requireArmaBlockUnchanged(t, blockPath, tc.block)
		})
	}
}

// newAppTestBlock returns a marshaled ARMA config block of an orderer org,
// nodeTestOrg, and of an application org, appTestExistingOrg.
func newAppTestBlock(t *testing.T) []byte {
	t.Helper()
	return newPartiesTestBlock(t, cryptogen.ConfigBlockParameters{
		Organizations: []cryptogen.OrganizationParameters{
			removeTestOrg(nodeTestOrg, nodeTestParty),
			appTestPeerOrg(appTestExistingOrg),
		},
	})
}

// newAppTestOrgGroup returns the config group of the application org
// appTestNewOrg, as a config block generated for it stores the group: its
// policies and its MSP, with the certificates of its own crypto material.
func newAppTestOrgGroup(t *testing.T) *cb.ConfigGroup {
	t.Helper()
	block := newPartiesTestBlock(t, cryptogen.ConfigBlockParameters{
		Organizations: []cryptogen.OrganizationParameters{
			removeTestOrg(nodeTestOrg, nodeTestParty),
			appTestPeerOrg(appTestNewOrg),
		},
	})
	application := nodeTestConfigOf(t, block).GetChannelGroup().GetGroups()[channelconfig.ApplicationGroupKey]
	org := application.GetGroups()[appTestNewOrg]
	require.NotNil(t, org)
	return org
}

// appTestPeerOrg returns the parameters of an application org of two peers.
func appTestPeerOrg(name string) cryptogen.OrganizationParameters {
	return cryptogen.OrganizationParameters{
		Name:   name,
		Domain: name + ".com",
		PeerNodes: []cryptogen.Node{
			{CommonName: appTestPeer0, Hostname: appTestPeer0 + "." + name},
			{CommonName: appTestPeer1, Hostname: appTestPeer1 + "." + name},
		},
	}
}

// withAppTestMSPName returns a copy of the org whose Fabric MSP is named mspID.
func withAppTestMSPName(t *testing.T, org *cb.ConfigGroup, mspID string) *cb.ConfigGroup {
	t.Helper()
	renamed := proto.CloneOf(org)
	setPartyTestMSPName(t, renamed, mspID)
	return renamed
}

// appTestDefinition renders the organization definition of an org: its name
// and its config group, in the form `fxadmin decode` prints the group, as YAML
// or JSON.
func appTestDefinition(t *testing.T, name string, org *cb.ConfigGroup, format string) string {
	t.Helper()
	definition := map[string]any{
		appTestNameKey:        name,
		appTestConfigGroupKey: protolatorMapOf(t, &peerext.DynamicApplicationOrgGroup{ConfigGroup: org}),
	}
	if format == appTestFormatJSON {
		content, err := json.Marshal(definition)
		require.NoError(t, err)
		return string(content)
	}
	return marshalDefinition(t, definition)
}

// writeAppTestDefinition writes the definition as peer2.yaml in a fresh
// directory and returns its path.
func writeAppTestDefinition(t *testing.T, definition string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), appTestNewOrg+".yaml")
	require.NoError(t, os.WriteFile(path, []byte(definition), 0o600))
	return path
}

// appTestREADMEDefinition is the organization definition of appTestNewOrg as
// the fxadmin README writes it. Each %s is a certificate, base64-encoded PEM:
// the MSP's root certificate, its admin certificate, its TLS root certificate,
// and two known certificates.
const appTestREADMEDefinition = `# peer2.yaml
name: peer2                          # the org's key under Application

config_group:                        # -> Application/groups/<name>
  version: "0"
  mod_policy: Admins
  groups: {}

  policies:                          # Readers, Writers, Admins, Endorsement
    Readers:
      version: "0"
      mod_policy: Admins
      policy:
        type: 1                      # 1 = SIGNATURE
        value:                       # OR('peer2.member')
          version: 0
          identities:
            - principal_classification: ROLE
              principal:
                msp_identifier: peer2
                role: MEMBER
          rule:
            n_out_of:
              n: 1
              rules:
                - signed_by: 0

    Writers:
      version: "0"
      mod_policy: Admins
      policy:
        type: 1
        value:                       # OR('peer2.member')
          version: 0
          identities:
            - principal_classification: ROLE
              principal:
                msp_identifier: peer2
                role: MEMBER
          rule:
            n_out_of:
              n: 1
              rules:
                - signed_by: 0

    Admins:
      version: "0"
      mod_policy: Admins
      policy:
        type: 1
        value:                       # OR('peer2.admin')
          version: 0
          identities:
            - principal_classification: ROLE
              principal:
                msp_identifier: peer2
                role: ADMIN
          rule:
            n_out_of:
              n: 1
              rules:
                - signed_by: 0

    Endorsement:
      version: "0"
      mod_policy: Admins
      policy:
        type: 1
        value:                       # OR('peer2.member')
          version: 0
          identities:
            - principal_classification: ROLE
              principal:
                msp_identifier: peer2
                role: MEMBER
          rule:
            n_out_of:
              n: 1
              rules:
                - signed_by: 0

  values:
    MSP:
      version: "0"
      mod_policy: Admins
      value:
        type: 0                      # 0 = FABRIC
        config:                      # FabricMSPConfig
          name: peer2                # MSP ID; the msp_identifier of the policies
          root_certs:
            - %s
          intermediate_certs: []
          admins:
            - %s
          revocation_list: []
          signing_identity: null
          organizational_unit_identifiers: []
          crypto_config:
            signature_hash_family: SHA2
            identity_identifier_hash_function: SHA256
          tls_root_certs:
            - %s
          tls_intermediate_certs: []
          fabric_node_ous: null
          known_certs:
            - %s
            - %s
`

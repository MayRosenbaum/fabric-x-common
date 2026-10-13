/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package modify

import (
	"fmt"
	"os"
	"slices"

	"github.com/cockroachdb/errors"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"go.yaml.in/yaml/v3"

	"github.com/Chaguar/common/common/channelconfig"
	"github.com/Chaguar/common/protolator/protoext/peerext"
)

// Keys of an organization definition: the org's name, its key under the
// Application group, and its config group, in the form it has in the config
// block.
const (
	orgDefNameKey        = "name"
	orgDefConfigGroupKey = "config_group"
)

// AppAdd implements `fxadmin modify app add`. The organization definition at
// orgPath holds the org's name, its key under the Application group, and its
// config group, in the form it has in the config block: its policies and its
// MSP, with every certificate given as base64-encoded PEM. The group is added
// as is, under that name, to the Application group of the config block at
// blockPath, which is rewritten in place; nothing is filled in. An org whose
// name is already in the Application group is rejected.
func (*Handler) AppAdd(orgPath, blockPath string) error {
	logger.Debugf("modify app add: org=%s block=%s", orgPath, blockPath)

	name, org, err := loadAppOrgDefinition(orgPath)
	if err != nil {
		return err
	}
	block, err := readAndDecodeConfigBlock(blockPath)
	if err != nil {
		return err
	}
	application := block.Config().GetChannelGroup().GetGroups()[channelconfig.ApplicationGroupKey]
	if application == nil {
		return errors.Newf("the config has no %s group", channelconfig.ApplicationGroupKey)
	}
	if _, exists := application.GetGroups()[name]; exists {
		return errors.Newf("application org %q already exists", name)
	}
	if application.Groups == nil {
		application.Groups = make(map[string]*cb.ConfigGroup)
	}
	application.Groups[name] = org

	if err := validateEncodeAndWriteConfigBlock(block, blockPath); err != nil {
		return err
	}
	logger.Infof("added application org %s to %s", name, blockPath)
	return nil
}

// loadAppOrgDefinition reads the organization definition YAML at path and
// returns the org's name and config group. The definition has exactly two
// keys: the name, and the config group, which is decoded as is.
func loadAppOrgDefinition(path string) (string, *cb.ConfigGroup, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", nil, errors.Wrapf(err, "failed to read organization definition %q", path)
	}
	definition := map[string]any{}
	if err = yaml.Unmarshal(content, &definition); err != nil {
		return "", nil, errors.Wrapf(err, "failed to parse organization definition %q", path)
	}
	for key := range definition {
		if !slices.Contains([]string{orgDefNameKey, orgDefConfigGroupKey}, key) {
			return "", nil, errors.Newf("organization definition %q has an unknown key %q: want %q and %q",
				path, key, orgDefNameKey, orgDefConfigGroupKey)
		}
	}
	name, ok := definition[orgDefNameKey].(string)
	if !ok || name == "" {
		return "", nil, errors.Newf("organization definition %q has no %s", path, orgDefNameKey)
	}
	group, ok := definition[orgDefConfigGroupKey].(map[string]any)
	if !ok {
		return "", nil, errors.Newf("organization definition %q has no %s", path, orgDefConfigGroupKey)
	}

	org := &cb.ConfigGroup{}
	if err := decodeProtolatorValue(group, &peerext.DynamicApplicationOrgGroup{ConfigGroup: org}); err != nil {
		return "", nil, fmt.Errorf("failed to decode the %s of organization definition %q: %w",
			orgDefConfigGroupKey, path, err)
	}
	return name, org, nil
}

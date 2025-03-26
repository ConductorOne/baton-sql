package bsql

import (
	"context"
	"errors"
	"fmt"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
)

var (
	ErrNoAccountProvisioningDefined = errors.New("no account provisioning defined")
)

func (c Config) extractTraits(rtID string) ([]v2.ResourceType_Trait, error) {
	rt, ok := c.ResourceTypes[rtID]
	if !ok {
		return nil, fmt.Errorf("resource type %s not found in config", rtID)
	}

	if rt.List == nil {
		return nil, fmt.Errorf("resource type %s has no listing defined", rtID)
	}

	if rt.List.Map == nil {
		return nil, fmt.Errorf("resource type %s has no listing map defined", rtID)
	}

	if rt.List.Map.Traits == nil {
		return nil, nil
	}

	var traits []v2.ResourceType_Trait

	if rt.List.Map.Traits.User != nil {
		traits = append(traits, v2.ResourceType_TRAIT_USER)
	}

	if rt.List.Map.Traits.Group != nil {
		traits = append(traits, v2.ResourceType_TRAIT_GROUP)
	}

	if rt.List.Map.Traits.Role != nil {
		traits = append(traits, v2.ResourceType_TRAIT_ROLE)
	}

	if rt.List.Map.Traits.App != nil {
		traits = append(traits, v2.ResourceType_TRAIT_APP)
	}

	return traits, nil
}

func (c Config) GetResourceTypes(ctx context.Context) ([]*v2.ResourceType, error) {
	var resourceTypes []*v2.ResourceType
	for rtID, rt := range c.ResourceTypes {
		traits, err := c.extractTraits(rtID)
		if err != nil {
			return nil, err
		}

		resourceType := &v2.ResourceType{
			Id:          rtID,
			DisplayName: rt.Name,
			Description: rt.Description,
			Traits:      traits,
		}

		if rt.SkipEntitlementsAndGrants {
			annos := annotations.Annotations(resourceType.Annotations)
			annos.Update(&v2.SkipEntitlementsAndGrants{})
			resourceType.Annotations = annos
		}
		resourceTypes = append(resourceTypes, resourceType)
	}
	return resourceTypes, nil
}

func (c Config) GetResourceType(ctx context.Context, rtID string) (*v2.ResourceType, error) {
	traits, err := c.extractTraits(rtID)
	if err != nil {
		return nil, err
	}

	rt, ok := c.ResourceTypes[rtID]
	if !ok {
		return nil, fmt.Errorf("resource type %s not found in config", rtID)
	}

	ret := &v2.ResourceType{
		Id:          rtID,
		DisplayName: rt.Name,
		Description: rt.Description,
		Traits:      traits,
	}

	if rt.SkipEntitlementsAndGrants {
		annos := annotations.Annotations(ret.Annotations)
		annos.Update(&v2.SkipEntitlementsAndGrants{})
		ret.Annotations = annos
	}

	return ret, nil
}

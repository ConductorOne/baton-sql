package helpers

import (
	"errors"
	"strings"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
)

// splitEntitlementID splits the entitlement ID into the resource type, resource ID, and entitlement ID.
func SplitEntitlementID(entitlement *v2.Entitlement) (string, string, string, error) {
	parts := strings.SplitN(entitlement.GetId(), ":", 3)
	if len(parts) != 3 {
		return "", "", "", errors.New("invalid entitlement ID")
	}

	resourceType := parts[0]
	resourceID := parts[1]
	entitlementID := parts[2]

	return resourceType, resourceID, entitlementID, nil
}

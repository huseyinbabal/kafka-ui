package kafka

import (
	"context"
	"sort"
	"strings"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
)

// ACL is one access control entry. Enum fields use Kafka's names, e.g.
// resourceType TOPIC, patternType LITERAL, operation READ, permission ALLOW.
type ACL struct {
	Principal    string `json:"principal"`
	Host         string `json:"host"`
	ResourceType string `json:"resourceType"`
	ResourceName string `json:"resourceName"`
	PatternType  string `json:"patternType"`
	Operation    string `json:"operation"`
	Permission   string `json:"permission"`
}

// ListACLs returns every ACL on the cluster.
func ListACLs(ctx context.Context, adm *kadm.Client) ([]ACL, error) {
	rs, err := adm.DescribeACLs(ctx, listAllFilter())
	if err != nil {
		return nil, err
	}
	seen := map[ACL]bool{}
	out := []ACL{}
	for _, r := range rs {
		if r.Err != nil {
			return nil, withMessage(r.Err, r.ErrMessage)
		}
		for _, d := range r.Described {
			a := ACL{
				Principal: d.Principal, Host: d.Host, ResourceType: d.Type.String(), ResourceName: d.Name,
				PatternType: d.Pattern.String(), Operation: d.Operation.String(), Permission: d.Permission.String(),
			}
			if !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ResourceType != b.ResourceType {
			return a.ResourceType < b.ResourceType
		}
		if a.ResourceName != b.ResourceName {
			return a.ResourceName < b.ResourceName
		}
		return a.Principal < b.Principal
	})
	return out, nil
}

func listAllFilter() *kadm.ACLBuilder {
	return kadm.NewACLs().
		AnyResource().
		ResourcePatternType(kadm.ACLPatternAny).
		Operations(kadm.OpAny).
		Allow().Deny().AllowHosts().DenyHosts()
}

// builder turns one ACL into a kadm builder for create or exact-match delete.
func (a ACL) builder() (*kadm.ACLBuilder, error) {
	bad := func(msg string) error { return apierr.BadRequest(msg, nil) }
	if !strings.Contains(a.Principal, ":") {
		return nil, bad("principal must look like User:alice")
	}
	if a.Host == "" {
		a.Host = "*"
	}
	rt, err := kmsg.ParseACLResourceType(a.ResourceType)
	if err != nil {
		return nil, bad("invalid resourceType")
	}
	pt, err := kmsg.ParseACLResourcePatternType(a.PatternType)
	if err != nil || (pt != kadm.ACLPatternLiteral && pt != kadm.ACLPatternPrefixed) {
		return nil, bad("patternType must be LITERAL or PREFIXED")
	}
	op, err := kmsg.ParseACLOperation(a.Operation)
	if err != nil || op == kadm.OpAny || op == kmsg.ACLOperationUnknown {
		return nil, bad("invalid operation")
	}
	perm, err := kmsg.ParseACLPermissionType(a.Permission)
	if err != nil || (perm != kmsg.ACLPermissionTypeAllow && perm != kmsg.ACLPermissionTypeDeny) {
		return nil, bad("permission must be ALLOW or DENY")
	}

	b := kadm.NewACLs()
	switch rt {
	case kmsg.ACLResourceTypeTopic:
		b.Topics(a.ResourceName)
	case kmsg.ACLResourceTypeGroup:
		b.Groups(a.ResourceName)
	case kmsg.ACLResourceTypeTransactionalId:
		b.TransactionalIDs(a.ResourceName)
	case kmsg.ACLResourceTypeCluster:
		b.Clusters()
	default:
		return nil, bad("resourceType must be TOPIC, GROUP, TRANSACTIONAL_ID or CLUSTER")
	}
	if rt != kmsg.ACLResourceTypeCluster && a.ResourceName == "" {
		return nil, bad("resourceName is required")
	}
	b.ResourcePatternType(pt).Operations(op)
	if perm == kmsg.ACLPermissionTypeAllow {
		b.Allow(a.Principal).AllowHosts(a.Host)
	} else {
		b.Deny(a.Principal).DenyHosts(a.Host)
	}
	return b, nil
}

func CreateACL(ctx context.Context, adm *kadm.Client, a ACL) error {
	b, err := a.builder()
	if err != nil {
		return err
	}
	if err := b.ValidateCreate(); err != nil {
		return apierr.BadRequest(err.Error(), err)
	}
	rs, err := adm.CreateACLs(ctx, b)
	if err != nil {
		return err
	}
	for _, r := range rs {
		if r.Err != nil {
			return withMessage(r.Err, r.ErrMessage)
		}
	}
	return nil
}

// DeleteACL deletes exactly the given ACL and returns how many entries matched.
func DeleteACL(ctx context.Context, adm *kadm.Client, a ACL) (int, error) {
	b, err := a.builder()
	if err != nil {
		return 0, err
	}
	if err := b.ValidateDelete(); err != nil {
		return 0, apierr.BadRequest(err.Error(), err)
	}
	rs, err := adm.DeleteACLs(ctx, b)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rs {
		if r.Err != nil {
			return n, withMessage(r.Err, r.ErrMessage)
		}
		n += len(r.Deleted)
	}
	return n, nil
}

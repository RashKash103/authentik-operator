package authentik

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// defaultPageSize bounds the list calls the resolvers make. Every resolver
// filters server-side on an exact field, so a page this size is only ever
// reached by a genuinely ambiguous reference — and one extra match is all that
// is needed to detect ambiguity.
const defaultPageSize int32 = 100

// uuidPattern matches a canonical RFC 4122 UUID, with or without hyphens, as
// authentik accepts both forms for its primary keys.
var uuidPattern = regexp.MustCompile(
	`^(?i:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[0-9a-f]{32})$`)

// IsUUID reports whether value is already an authentik primary key rather than
// a human-readable name. CRDs accept either, and a UUID is passed through
// untouched instead of being looked up.
func IsUUID(value string) bool {
	return uuidPattern.MatchString(strings.TrimSpace(value))
}

// ResolveFlow implements Client. It maps a flow slug to the flow's UUID.
func (c *client) ResolveFlow(ctx context.Context, slug string) (string, error) {
	const op = "resolve flow"

	return c.resolveUUID(ctx, op, KindFlow, slug, func(ctx context.Context, name string) ([]string, error) {
		list, resp, err := c.api.FlowsAPI.FlowsInstancesList(ctx).
			Slug(name).
			PageSize(c.pageSize).
			Execute()
		if err != nil {
			return nil, mapResponse(op, resp, err)
		}
		if list == nil {
			return nil, transientErrorf(op, "empty list response")
		}
		pks := make([]string, 0, len(list.Results))
		for _, flow := range list.Results {
			// The slug filter is exact server-side; re-check so a future
			// loosening of the filter cannot silently widen a match.
			if flow.Slug == name {
				pks = append(pks, flow.Pk)
			}
		}
		return pks, nil
	})
}

// ResolvePropertyMapping implements Client. It maps a property mapping name to
// its UUID, searching across every property mapping type.
func (c *client) ResolvePropertyMapping(ctx context.Context, name string) (string, error) {
	const op = "resolve property mapping"

	return c.resolveUUID(ctx, op, KindPropertyMapping, name, func(ctx context.Context, name string) ([]string, error) {
		list, resp, err := c.api.PropertymappingsAPI.PropertymappingsAllList(ctx).
			Name(name).
			PageSize(c.pageSize).
			Execute()
		if err != nil {
			return nil, mapResponse(op, resp, err)
		}
		if list == nil {
			return nil, transientErrorf(op, "empty list response")
		}
		pks := make([]string, 0, len(list.Results))
		for _, mapping := range list.Results {
			if mapping.Name == name {
				pks = append(pks, mapping.Pk)
			}
		}
		return pks, nil
	})
}

// ResolvePropertyMappings implements Client.
func (c *client) ResolvePropertyMappings(ctx context.Context, names []string) ([]string, error) {
	resolved := make([]string, 0, len(names))
	for _, name := range names {
		pk, err := c.ResolvePropertyMapping(ctx, name)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, pk)
	}
	return resolved, nil
}

// ResolveCertificateKeyPair implements Client. It maps a certificate-key pair
// name to its UUID.
func (c *client) ResolveCertificateKeyPair(ctx context.Context, name string) (string, error) {
	const op = "resolve certificate keypair"

	return c.resolveUUID(ctx, op, KindCertificateKeyPair, name, func(ctx context.Context, name string) ([]string, error) {
		list, resp, err := c.api.CryptoAPI.CryptoCertificatekeypairsList(ctx).
			Name(name).
			PageSize(c.pageSize).
			Execute()
		if err != nil {
			return nil, mapResponse(op, resp, err)
		}
		if list == nil {
			return nil, transientErrorf(op, "empty list response")
		}
		pks := make([]string, 0, len(list.Results))
		for _, pair := range list.Results {
			if pair.Name == name {
				pks = append(pks, pair.Pk)
			}
		}
		return pks, nil
	})
}

// ResolveServiceConnection implements Client. It maps an outpost service
// connection name to its UUID.
func (c *client) ResolveServiceConnection(ctx context.Context, name string) (string, error) {
	const op = "resolve service connection"

	return c.resolveUUID(ctx, op, KindServiceConnection, name, func(ctx context.Context, name string) ([]string, error) {
		list, resp, err := c.api.OutpostsAPI.OutpostsServiceConnectionsAllList(ctx).
			Name(name).
			PageSize(c.pageSize).
			Execute()
		if err != nil {
			return nil, mapResponse(op, resp, err)
		}
		if list == nil {
			return nil, transientErrorf(op, "empty list response")
		}
		pks := make([]string, 0, len(list.Results))
		for _, conn := range list.Results {
			if conn.Name == name {
				pks = append(pks, conn.Pk)
			}
		}
		return pks, nil
	})
}

// ResolveProvider implements Client. Providers are keyed by an integer primary
// key rather than a UUID, so a value that already parses as an integer is
// passed straight through.
//
// The providers endpoint offers no exact-name filter — only a fuzzy search — so
// the search result is narrowed to exact name matches here.
func (c *client) ResolveProvider(ctx context.Context, name string) (int32, error) {
	const op = "resolve provider"

	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return 0, notFoundError(op, KindProvider.String(), name)
	}
	if pk, err := strconv.ParseInt(trimmed, 10, 32); err == nil {
		return int32(pk), nil
	}

	if cached, ok := c.cache.Get(c.connID, KindProvider, trimmed); ok {
		if pk, err := strconv.ParseInt(cached, 10, 32); err == nil {
			return int32(pk), nil
		}
		// A malformed entry can only come from a bug; drop it and look again.
		c.cache.Invalidate(c.connID, KindProvider, trimmed)
	}

	matches, err := c.listProviderPKs(ctx, op, trimmed)
	if err != nil {
		return 0, err
	}
	switch len(matches) {
	case 0:
		c.cache.Invalidate(c.connID, KindProvider, trimmed)
		return 0, notFoundError(op, KindProvider.String(), trimmed)
	case 1:
		c.cache.Set(c.connID, KindProvider, trimmed, strconv.FormatInt(int64(matches[0]), 10))
		return matches[0], nil
	default:
		c.cache.Invalidate(c.connID, KindProvider, trimmed)
		return 0, ambiguousError(op, KindProvider.String(), trimmed, len(matches))
	}
}

// listProviderPKs returns the primary keys of every provider whose name is
// exactly name.
func (c *client) listProviderPKs(ctx context.Context, op, name string) ([]int32, error) {
	list, resp, err := c.api.ProvidersAPI.ProvidersAllList(ctx).
		Search(name).
		PageSize(c.pageSize).
		Execute()
	if err != nil {
		return nil, mapResponse(op, resp, err)
	}
	if list == nil {
		return nil, transientErrorf(op, "empty list response")
	}

	var matches []int32
	for _, provider := range list.Results {
		if provider.Name == name {
			matches = append(matches, provider.Pk)
		}
	}
	return matches, nil
}

// lookupFunc performs the live lookup for one reference kind, returning every
// primary key that matched exactly.
type lookupFunc func(ctx context.Context, name string) ([]string, error)

// resolveUUID is the shared resolution path for every UUID-keyed reference.
//
// The semantics it enforces are the point of the whole file:
//   - a value that is already a UUID is passed through unresolved;
//   - exactly one match resolves, and is cached for one TTL;
//   - zero matches is ErrNotFound, so the caller can requeue and wait for the
//     object to be created;
//   - more than one match is a hard error. An ambiguous reference is never
//     silently narrowed to whichever object happened to sort first.
func (c *client) resolveUUID(
	ctx context.Context,
	op string,
	kind RefKind,
	name string,
	lookup lookupFunc,
) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", notFoundError(op, kind.String(), name)
	}
	if IsUUID(trimmed) {
		return trimmed, nil
	}
	if cached, ok := c.cache.Get(c.connID, kind, trimmed); ok {
		return cached, nil
	}

	matches, err := lookup(ctx, trimmed)
	if err != nil {
		return "", err
	}

	switch len(matches) {
	case 0:
		// Invalidate on a miss: a flow created moments ago must be picked up on
		// the next reconcile, not after the TTL of a stale entry.
		c.cache.Invalidate(c.connID, kind, trimmed)
		return "", notFoundError(op, kind.String(), trimmed)
	case 1:
		c.cache.Set(c.connID, kind, trimmed, matches[0])
		return matches[0], nil
	default:
		c.cache.Invalidate(c.connID, kind, trimmed)
		return "", ambiguousError(op, kind.String(), trimmed, len(matches))
	}
}

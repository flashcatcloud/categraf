package gnmi

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	gnmiLib "github.com/openconfig/gnmi/proto/gnmi"
	gnmiExt "github.com/openconfig/gnmi/proto/gnmi_ext"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	jnprHeader "flashcat.cloud/categraf/inputs/gnmi/extensions/jnpr_gnmi_extention"
	"flashcat.cloud/categraf/pkg/choice"
	"flashcat.cloud/categraf/types"
)

const eidJuniperTelemetryHeader = 1

type handler struct {
	address             string
	aliases             map[*pathInfo]string
	tagsubs             []TagSubscription
	subs                []Subscription
	maxMsgSize          int
	emptyNameWarnShown  bool
	vendorExt           []string
	tagStore            *tagStore
	trace               bool
	canonicalFieldNames bool
	trimSlash           bool
	guessPathTag        bool

	sourceTag   string
	health      *sessionHealth
	tagWarnings tagWarningLimiter
}

// SubscribeGNMI and extract telemetry data
func (h *handler) subscribeGNMI(ctx context.Context, slist *types.SampleList, tlscfg *tls.Config, request *gnmiLib.SubscribeRequest) error {
	var generation uint64
	if h.health != nil {
		generation = h.health.beginAttempt()
		defer h.health.finish(generation)
	}
	var creds credentials.TransportCredentials
	if tlscfg != nil {
		creds = credentials.NewTLS(tlscfg)
	} else {
		creds = insecure.NewCredentials()
	}
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
	}

	if h.maxMsgSize > 0 {
		opts = append(opts, grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(h.maxMsgSize),
		))
	}

	client, err := grpc.DialContext(ctx, h.address, opts...)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}
	defer client.Close()

	subscribeClient, err := gnmiLib.NewGNMIClient(client).Subscribe(ctx)
	if err != nil {
		return fmt.Errorf("failed to setup subscription: %w", err)
	}

	// If io.EOF is returned, the stream may have ended and stream status
	// can be determined by calling Recv.
	if err := subscribeClient.Send(request); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to send subscription request: %w", err)
	}

	log.Printf("Connection to gNMI device %s established", h.address)

	// Used to report the status of the TCP connection to the device. If the
	// GNMI connection goes down, but TCP is still up this will still report
	// connected until the TCP connection times out.
	// connectStat := selfstat.Register("gnmi", "grpc_connection_status", map[string]string{"source": h.address})
	// connectStat.Set(1)

	defer log.Printf("Connection to gNMI device %s closed", h.address)

	for ctx.Err() == nil {
		reply, err := subscribeClient.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				return fmt.Errorf("aborted gNMI subscription: %w", err)
			}
			break
		}
		switch response := reply.Response.(type) {
		case *gnmiLib.SubscribeResponse_Error:
			return fmt.Errorf("gNMI subscription error code %d", response.Error.GetCode())
		case *gnmiLib.SubscribeResponse_SyncResponse:
			if response.SyncResponse && h.health != nil {
				h.health.observe(generation, time.Now(), false)
			}
		case *gnmiLib.SubscribeResponse_Update:
			if response.Update != nil && h.health != nil {
				h.health.observe(generation, time.Now(), len(response.Update.Update) > 0 || len(response.Update.Delete) > 0)
			}
		}
		if h.trace {
			buf, err := protojson.Marshal(reply)
			if err != nil {
				log.Printf("Marshal failed: %v", err)
			} else {
				log.Printf("Got update_%v: %s", reply.GetUpdate().GetTimestamp(), string(buf))
			}
		}
		if response, ok := reply.Response.(*gnmiLib.SubscribeResponse_Update); ok {
			h.handleSubscribeResponseUpdate(slist, response, reply.GetExtension())
		}
	}

	// connectStat.Set(0)
	return nil
}

// Handle SubscribeResponse_Update message from gNMI and parse contained telemetry data
func (h *handler) handleSubscribeResponseUpdate(slist *types.SampleList, response *gnmiLib.SubscribeResponse_Update, extension []*gnmiExt.Extension) {
	responseUpdate := response.Update
	timestamp := time.Unix(0, responseUpdate.Timestamp)

	// Extract tags from potential extension in the update notification
	headerTags := make(map[string]string)
	for _, ext := range extension {
		currentExt := ext.GetRegisteredExt().Msg
		if currentExt == nil {
			break
		}

		switch ext.GetRegisteredExt().Id {
		case eidJuniperTelemetryHeader:
			// Juniper Header extention
			// Decode it only if user requested it
			if choice.Contains("juniper_header", h.vendorExt) {
				juniperHeader := &jnprHeader.GnmiJuniperTelemetryHeaderExtension{}
				if err := proto.Unmarshal(currentExt, juniperHeader); err != nil {
					log.Printf("unmarshal gnmi Juniper Header extension failed: %v", err)
				} else {
					// Add only relevant Tags from the Juniper Header extension.
					// These are required for aggregation
					headerTags["component_id"] = strconv.FormatUint(uint64(juniperHeader.GetComponentId()), 10)
					headerTags["component"] = juniperHeader.GetComponent()
					headerTags["sub_component_id"] = strconv.FormatUint(uint64(juniperHeader.GetSubComponentId()), 10)
				}
			}
		default:
			continue
		}
	}

	// Extract the path part valid for the whole set of updates if any
	prefix := newInfoFromPath(responseUpdate.Prefix)

	// Add info to the tags
	headerTags[h.sourceTag], _, _ = net.SplitHostPort(h.address)
	if !prefix.empty() {
		headerTags["path"] = prefix.String()
	}

	// Process and remove tag-updates from the response first so we can
	// add all available tags to the metrics later.
	var valueFields []updateField
	for _, update := range responseUpdate.Update {
		fullPath := prefix.append(update.Path)
		fields, err := newFieldsFromUpdate(fullPath, update)
		if err != nil {
			log.Printf("Processing update %v failed: %v", update, err)
		}

		// Prepare tags from prefix
		tags := make(map[string]string, len(headerTags))
		for key, val := range headerTags {
			tags[key] = val
		}
		for key, val := range fullPath.Tags() {
			tags[key] = val
		}

		// TODO: Handle each field individually to allow in-JSON tags
		var tagUpdate bool
		selected, matchErr := h.selectTagSubscription(fullPath)
		if matchErr != nil {
			h.tagWarnings.warn("W! gNMI %s: %v", h.address, matchErr)
		}
		if selected >= 0 {
			tagSub := h.tagsubs[selected]
			log.Printf("Tag-subscription update for %q: %+v", tagSub.Name, update)
			if err := h.tagStore.insert(tagSub, fullPath, fields, tags); err != nil {
				log.Printf("E! Inserting tag failed: %v", err)
			}
			tagUpdate = true
		}
		if !tagUpdate {
			valueFields = append(valueFields, fields...)
		}
	}

	// Some devices do not provide a prefix, so do some guesswork based
	// on the paths of the fields
	if headerTags["path"] == "" && h.guessPathTag {
		if prefixPath := guessPrefixFromUpdate(valueFields); prefixPath != "" {
			headerTags["path"] = prefixPath
		}
	}

	// Parse individual update message and create measurements
	for _, field := range valueFields {
		// Prepare tags from prefix
		fieldTags := field.path.Tags()
		tags := make(map[string]string, len(headerTags)+len(fieldTags))
		for key, val := range headerTags {
			tags[key] = val
		}
		for key, val := range fieldTags {
			tags[key] = val
		}

		// Add the tags derived via tag-subscriptions
		for k, v := range h.tagStore.lookup(field.path, tags) {
			tags[k] = v
		}

		// Lookup alias for the metric
		aliasPath, name := h.lookupAlias(field.path)
		if name == "" {
			log.Printf("No measurement alias for gNMI path: %s", field.path)
			if !h.emptyNameWarnShown {
				log.Printf(emptyNameWarning, responseUpdate)
				h.emptyNameWarnShown = true
			}
		}

		// Group metrics
		fieldPath := field.path.String()
		key := strings.ReplaceAll(fieldPath, "-", "_")
		if h.canonicalFieldNames {
			// Strip the origin is any for the field names
			if parts := strings.SplitN(key, ":", 2); len(parts) == 2 {
				key = parts[1]
			}
		} else {
			if len(aliasPath) < len(key) && len(aliasPath) != 0 {
				// This may not be an exact prefix, due to naming style
				// conversion on the key.
				key = key[len(aliasPath)+1:]
			} else if len(aliasPath) >= len(key) {
				// Otherwise use the last path element as the field key.
				key = path.Base(key)
			}
		}
		if h.trimSlash {
			key = strings.TrimLeft(key, "/.")
		}
		if key == "" {
			log.Printf("E! Invalid empty path %q with alias %q", fieldPath, aliasPath)
			continue
		}
		prefix := inputName
		if strings.HasPrefix(name, inputName) {
			prefix = ""
		}
		value := field.value
		disableConcatenating := false
		var conversionErr error

		// Resolve the owning value subscription with origin precedence:
		// an explicit/inline origin that equals the field origin is a strong
		// match; an origin-less subscription is only a weak wildcard fallback.
		// A weak match must never override a strong one, otherwise a generic
		// origin-less subscription could flip disable_concatenation/conversion
		// of a specific origin subscription (double-concatenated metric names).
		buildSubPath := func(sub *Subscription) *pathInfo {
			subPath := newInfoFromString(sub.Path)
			if sub.Origin != "" {
				subPath.origin = sub.Origin
				subPath.explicitOrigin = true
			}
			return subPath
		}
		var matched *Subscription
		for i := range h.subs {
			sub := &h.subs[i]
			if ok, strong := buildSubPath(sub).subPathMatch(field.path); ok && strong {
				matched = sub // later strong match wins, preserving prior behavior
			}
		}
		if matched == nil {
			for i := range h.subs {
				sub := &h.subs[i]
				if ok, strong := buildSubPath(sub).subPathMatch(field.path); ok && !strong {
					matched = sub
				}
			}
		}
		if matched != nil {
			disableConcatenating = matched.DisableConcatenation
			if matched.Conversion == "ieeefloat32" {
				// Skip the field on a decode failure instead of emitting a
				// misleading zero value.
				converted, err := convertIEEEFloat32(field.value)
				if err != nil {
					conversionErr = fmt.Errorf("subscription %q ieeefloat32 conversion for %s failed: %w", matched.Name, fieldPath, err)
				} else {
					value = converted
				}
			}
		}
		if conversionErr != nil {
			log.Printf("E! %v", conversionErr)
			continue
		}
		if !disableConcatenating {
			name = name + "_" + key
		}

		sample := types.NewSample(prefix, name, value, tags).SetTime(timestamp)
		slist.PushFront(sample)
	}

}

// Try to find the alias for the given path
type aliasCandidate struct {
	path, alias string
	strong      bool
}

func (h *handler) lookupAlias(info *pathInfo) (aliasPath, alias string) {
	candidates := make([]aliasCandidate, 0)
	anyStrong := false
	for i, a := range h.aliases {
		ok, strong := i.subPathMatch(info)
		if !ok {
			continue
		}
		// Use the field's origin spelling when trimming the matched prefix.
		// Compatible standalone and inline origins can have different lengths.
		matchedPath := &pathInfo{origin: info.origin, segments: i.segments}
		candidates = append(candidates, aliasCandidate{path: matchedPath.String(), alias: a, strong: strong})
		anyStrong = anyStrong || strong
	}
	if len(candidates) == 0 {
		return "", ""
	}

	// Prefer origin-strong matches; only fall back to origin-less wildcard
	// aliases when no strong alias exists.
	if anyStrong {
		filtered := candidates[:0]
		for _, c := range candidates {
			if c.strong {
				filtered = append(filtered, c)
			}
		}
		candidates = filtered
	}

	// Reverse sort the candidates by path length so we can use the longest match
	sort.SliceStable(candidates, func(i, j int) bool {
		return len(candidates[i].path) > len(candidates[j].path)
	})

	return candidates[0].path, candidates[0].alias
}

func guessPrefixFromUpdate(fields []updateField) string {
	if len(fields) == 0 {
		return ""
	}
	if len(fields) == 1 {
		dir, _ := fields[0].path.split()
		return dir
	}
	commonPath := &pathInfo{
		origin:   fields[0].path.origin,
		segments: append([]string{}, fields[0].path.segments...),
	}
	for _, f := range fields[1:] {
		commonPath.keepCommonPart(f.path)
	}
	if commonPath.empty() {
		return ""
	}
	return commonPath.String()
}

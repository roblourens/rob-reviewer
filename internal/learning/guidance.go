package learning

import (
	"fmt"
	"slices"
	"strings"

	"github.com/roblourens/rob-reviewer/internal/review"
)

type Proposal struct {
	CaseID          string                           `json:"caseId"`
	MechanismFamily review.RegressionMechanismFamily `json:"mechanismFamily"`
}

func Proposals(cases []RegressionCase) []Proposal {
	var result []Proposal
	for _, regressionCase := range cases {
		if regressionCase.Coverage.Status != "instruction-gap" {
			continue
		}
		result = append(result, Proposal{
			CaseID:          regressionCase.ID,
			MechanismFamily: regressionCase.MechanismFamily,
		})
	}
	slices.SortFunc(result, func(left, right Proposal) int {
		if compared := strings.Compare(string(left.MechanismFamily), string(right.MechanismFamily)); compared != 0 {
			return compared
		}
		return strings.Compare(left.CaseID, right.CaseID)
	})
	return result
}

func ApplyProposals(content string, proposals []Proposal) (string, bool, error) {
	updated := strings.TrimRight(content, "\n")
	changed := false
	seenFamilies := make(map[review.RegressionMechanismFamily]struct{})
	for _, proposal := range proposals {
		if _, seen := seenFamilies[proposal.MechanismFamily]; seen {
			continue
		}
		seenFamilies[proposal.MechanismFamily] = struct{}{}
		heading, guidance, ok := predefinedGuidance(proposal.MechanismFamily)
		if !ok {
			return "", false, fmt.Errorf("unsupported learning proposal family %q", proposal.MechanismFamily)
		}
		marker := fmt.Sprintf("<!-- learned-family:%s -->", proposal.MechanismFamily)
		if strings.Contains(updated, marker) {
			continue
		}
		updated += fmt.Sprintf("\n\n%s\n## %s\n\n%s\n", marker, heading, guidance)
		changed = true
	}
	if updated != "" {
		updated = strings.TrimRight(updated, "\n") + "\n"
	}
	return updated, changed, nil
}

func predefinedGuidance(family review.RegressionMechanismFamily) (string, string, bool) {
	switch family {
	case review.RegressionRepeatedWork:
		return "Repeated work revealed by shipped fixes", "When a fix removes repeated work, reconstruct the pre-fix multiplicity explicitly. Check whether the introducing diff placed collection-wide work inside an incremental, per-item, per-event, or per-frame path, including work hidden behind helpers.", true
	case review.RegressionUnboundedRetention:
		return "Retention revealed by shipped fixes", "Trace closure, cache, listener, and owner reachability after the last active consumer disappears. A bounded container can still retain large per-entry graphs; multiply its capacity by realistic retained payload size and check lifecycle eviction.", true
	case review.RegressionEagerWork:
		return "Eager work revealed by shipped fixes", "Inspect eagerly constructed hidden sections and details. Report when a changed constructor starts heavyweight UI, model, filesystem, or network work before the feature is visible or requested, and identify the first-use boundary that could own lazy creation.", true
	case review.RegressionBoundaryFanout:
		return "Boundary fan-out revealed by shipped fixes", "Count effective subprocess, IPC, filesystem, database, and network operations after grouping. Check whether a changed loop turns one logical request into per-item boundary calls even when callers use Promise.all or other superficial concurrency.", true
	case review.RegressionMissingCoalescing:
		return "Missing coalescing revealed by shipped fixes", "Check work triggered by rapid successive versions or events before the expensive boundary. Establish whether cancellation occurs before issuance or only after work has started, and report changed debounce, batching, or in-flight joining that increases issued operations.", true
	case review.RegressionSynchronousUI:
		return "Synchronous UI work revealed by shipped fixes", "Trace reads and writes on resize, scroll, render, and input paths through browser layout and compositor boundaries. Look for changed geometry reads after writes, live filters on moving surfaces, and synchronous nested relayouts.", true
	case review.RegressionCacheLifecycle:
		return "Cache lifecycle revealed by shipped fixes", "Evaluate positive and negative cache states separately. Check whether misses, empty successes, failures, and not-yet-discovered resources are distinguishable, bounded, invalidated, and coalesced rather than retried per item or pinned indefinitely.", true
	case review.RegressionCleanupLifecycle:
		return "Cleanup lifecycle revealed by shipped fixes", "Trace cleanup ownership through failure, cancellation, replacement, and shutdown. Report when a changed path loses the last handle to a still-live resource, repeats expensive cleanup, or moves cleanup into contention with an important scenario.", true
	case review.RegressionSerialization:
		return "Serialization and allocation revealed by shipped fixes", "Check whether changed telemetry, logging, protocol, or storage paths sanitize, clone, encode, or buffer unbounded data before a downstream cap. Bound work before allocation and transformation whenever only a prefix can be consumed.", true
	case review.RegressionConcurrencyBurst:
		return "Concurrency bursts revealed by shipped fixes", "Distinguish per-key serialization from global concurrency. When one event schedules work for many independent keys, compute the aggregate resource burst and look for a shared limiter that bounds expensive creation, disposal, or remote operations.", true
	default:
		return "", "", false
	}
}

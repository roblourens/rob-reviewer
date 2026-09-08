# Learned regression mechanisms

This file contains host-generated review guidance derived from validated pairs of introducing and fixing pull requests. Entries are selected from predefined guidance templates; pull request text and model-authored prose are never copied here.

<!-- learned-family:cleanup-lifecycle -->
## Cleanup lifecycle revealed by shipped fixes

Trace cleanup ownership through failure, cancellation, replacement, and shutdown. Report when a changed path loses the last handle to a still-live resource, repeats expensive cleanup, or moves cleanup into contention with an important scenario.

<!-- learned-family:synchronous-ui-work -->
## Synchronous UI work revealed by shipped fixes

Trace reads and writes on resize, scroll, render, and input paths through browser layout and compositor boundaries. Look for changed geometry reads after writes, live filters on moving surfaces, and synchronous nested relayouts.

<!-- learned-family:eager-work -->
## Eager work revealed by shipped fixes

Inspect eagerly constructed hidden sections and details. Report when a changed constructor starts heavyweight UI, model, filesystem, or network work before the feature is visible or requested, and identify the first-use boundary that could own lazy creation.

<!-- learned-family:boundary-fanout -->
## Boundary fan-out revealed by shipped fixes

Count effective subprocess, IPC, filesystem, database, and network operations after grouping. Check whether a changed loop turns one logical request into per-item boundary calls even when callers use Promise.all or other superficial concurrency.

<!-- learned-family:repeated-work -->
## Repeated work revealed by shipped fixes

When a fix removes repeated work, reconstruct the pre-fix multiplicity explicitly. Check whether the introducing diff placed collection-wide work inside an incremental, per-item, per-event, or per-frame path, including work hidden behind helpers.

<!-- learned-family:serialization-allocation -->
## Serialization and allocation revealed by shipped fixes

Check whether changed telemetry, logging, protocol, or storage paths sanitize, clone, encode, or buffer unbounded data before a downstream cap. Bound work before allocation and transformation whenever only a prefix can be consumed.

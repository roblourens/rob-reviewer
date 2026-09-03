# Learned regression mechanisms

This file contains host-generated review guidance derived from validated pairs of introducing and fixing pull requests. Entries are selected from predefined guidance templates; pull request text and model-authored prose are never copied here.

<!-- learned-family:cleanup-lifecycle -->
## Cleanup lifecycle revealed by shipped fixes

Trace cleanup ownership through failure, cancellation, replacement, and shutdown. Report when a changed path loses the last handle to a still-live resource, repeats expensive cleanup, or moves cleanup into contention with an important scenario.

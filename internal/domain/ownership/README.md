# internal/domain/ownership

Classifies each catalog package into exactly one ownership state (`absent`, `foreign`, `managed`, `adopted`, `drifted`, `duplicate`, `conflict`, `unavailable-here`) from detections, the inventory record, the conflict pre-flight and availability. See `Classify` for the precedence.

| File | Contents |
|---|---|
| `ownership.go` | `State`, `Detection`, `DetectorID`, `DetectionStatus`, `PackageStatus`, `Classify`, `DetectedThrough` |

**Architecture.** Domain layer: no I/O. Imports `catalog`, `inventory`, `plan`, `version`. Used by the scan, planning, doctor and status use cases and by detector adapters (which produce `Detection`s).

Related: [data dictionary](../../../specs/261114-software-factory-manager/data-dictionary.md), [architecture](../../../specs/261114-software-factory-manager/architecture.md). Back to [internal/domain](../README.md).

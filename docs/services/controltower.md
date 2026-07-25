# Control Tower

**Status:** shipped (honest stub)

Control Tower landing-zone APIs stay intentionally thin. Organizations list/policy APIs cover OU and SCP evidence for lab governance forensics; this surface does not invent a landing-zone control plane.

## Implemented

| Action | Behavior |
|--------|----------|
| `ListLandingZones` | Returns an empty `landingZones` list |
| `GetLandingZone` | Returns `ResourceNotFoundException` |

Identity authz applies. No create/enable/update landing-zone APIs.

## Out of lab scope

- Landing zone create/enable, controls catalog, Account Factory, and real Control Tower orchestration (out of lab scope)

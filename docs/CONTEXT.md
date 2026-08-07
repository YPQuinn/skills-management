# Local Skill Management

The domain covers maintaining an authoritative local collection of Agent Skills and projecting selected Skills into local AI Agent environments.

## Language

**Skill**:
A directory conforming to the Agent Skills convention, identified within the product by a globally unique slug and containing `SKILL.md` plus any supporting assets.
_Avoid_: Plugin, prompt, extension

**Skill Store**:
The authoritative local collection of managed Skills. Copies elsewhere are not authoritative.
_Avoid_: Truth source, skills directory, registry

**Source**:
An upstream location from which one or more Skills can be imported and checked for changes. A Source is not part of the Skill Store.
_Avoid_: Skill Store, destination, registry

**Group**:
A named, non-owning collection of Skills. A Skill may belong to multiple Groups.
_Avoid_: Folder, category directory

**Target**:
A specific user-level, project-level, or custom Agent location that receives selected Skills from the Skill Store.
_Avoid_: Source, install directory, destination directory

**Assignment**:
A declaration that a Skill or Group should be present at a Target. Multiple Assignments combine into the Target's desired Skill set.
_Avoid_: Copy, installation

**Distribution**:
The reconciliation of a Target with its Assignments. Distributed Skills remain authoritative in the Skill Store.
_Avoid_: Import, synchronization

**Distribution Status**:
The desired and observed presence of a Skill at a Target, including the latest reconciliation outcome.
_Avoid_: Installation status, sync status

**Synchronization**:
The explicit comparison and one-way retrieval of Source changes into the Skill Store.
_Avoid_: Distribution, publishing

**Sync Status**:
The relationship between a Skill in the Skill Store and its bound Source, including whether either side has changed since their last successful synchronization.
_Avoid_: Distribution status, version

**Conflict**:
A state where synchronization or distribution cannot proceed safely without choosing which existing content should prevail.
_Avoid_: Error, overwrite

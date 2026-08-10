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

**Source Inventory**:
The last observed set of valid upstream Skill entries exposed by a Source, each identified by its relative directory.
_Avoid_: Skill Store, catalog

**Source Binding**:
The association from one managed Skill to one Source Inventory entry. Removing the association does not remove the Skill.
_Avoid_: Assignment, installation

**Group**:
A named, non-owning collection of Skills. A Skill may belong to multiple Groups.
_Avoid_: Folder, category directory

**Target**:
A specific user-level, project-level, or custom Agent location that receives selected Skills from the Skill Store.
_Avoid_: Source, install directory, destination directory

**Target Adapter**:
A built-in rule that detects a supported Agent and resolves its user-level or project-level Skills container when creating a Target. It is a path-discovery aid, not the Target's identity or owner.
_Avoid_: Target, Assignment, Agent installation

**Assignment**:
A declaration that a Skill or Group should be present at a Target. Multiple Assignments combine into the Target's desired Skill set.
_Avoid_: Copy, installation

**Distribution**:
The reconciliation of a Target with its Assignments. Distributed Skills remain authoritative in the Skill Store.
_Avoid_: Import, synchronization

**Managed Link**:
A Target symlink that Skill Manager created or explicitly adopted and still owns according to its recorded identity. Only a Managed Link may be changed or removed by Distribution.
_Avoid_: Any symlink, installed Skill, copied Skill

**Distribution Status**:
The desired and last-observed presence of a Skill at a Target, together with observation freshness and the latest reconciliation outcome.
_Avoid_: Installation status, sync status

**Synchronization**:
The explicit comparison and one-way retrieval of Source changes into the Skill Store.
_Avoid_: Distribution, publishing

**Synchronization Baseline**:
The accepted Source content against which current Source and Skill Store content are compared during synchronization. It is comparison state, not user-visible version history.
_Avoid_: Version, backup, rollback snapshot

**Sync Status**:
The observed content relationship between a Skill in the Skill Store and its bound Source relative to their Synchronization Baseline. Source availability and observation freshness are separate from this relationship.
_Avoid_: Distribution status, version, availability

**Conflict**:
A state where synchronization or distribution cannot proceed safely without choosing which existing content should prevail.
_Avoid_: Error, overwrite

# Define the Distribution reconciliation lifecycle

Type: grilling
Status: resolved
Blocked by: 04

## Question

What exact desired-state calculation, filesystem inspection, status transitions, managed-link ownership rules, conflict handling, cleanup, and retry behavior should reconcile Assignments into Target symlinks?

## Answer

### Desired Skill set

A Target's desired Skill set is recomputed from current state whenever it is inspected or reconciled. It is the union of directly assigned Skills and the current members of all assigned Groups, deduplicated by immutable Skill ID. Every contributing Assignment remains available as explanation even when several Assignments select the same Skill.

Assignment and Group-membership changes update desired state immediately but never mutate a Target automatically. Distribution remains explicit. The desired link name is always the current Skill slug: `<target>/<slug>`. A desired Skill that is missing or invalid in the Skill Store stays desired and blocked rather than disappearing from status. Referenced-Skill deletion retains the map's blocking and explicit-cleanup rules.

### Link representation and ownership

Skill Manager creates absolute symbolic links from `<target>/<slug>` to the normalized physical `<store>/<slug>` path. It writes no persistent marker or sidecar into a Target. Creation and removal may use a transient, random 0700 directory recorded in the operation's durable intent; completed operations remove that directory only when empty.

A **Managed Link** is established only when Skill Manager creates a link successfully or the user explicitly adopts an eligible existing link. SQLite records Target ID, Skill ID, link path, the exact raw link target, physical symlink identity (device, inode, and modification time), and creation or adoption time. A current entry remains provably managed only while the ownership record exists, the path is still a symlink, and its raw target and physical identity still match that record. Physical resolution is checked separately for correctness and safety.

If a user replaces the entry with a file, directory, or different symlink, the old record does not authorize Skill Manager to modify the replacement. Loss of SQLite likewise makes existing Target links unmanaged; ownership is never reconstructed from path or naming alone.

An unmanaged symlink can be explicitly **Adopted** only when fresh physical resolution proves that it points exactly to the currently desired Store Skill. Adoption rechecks the path, records its existing raw target and physical identity, and does not rewrite the link. Files, directories, wrong-target links, and links outside the Store cannot be adopted. There is no automatic adoption.

### Target path safety and creation

Registering a Target remains read-only. The first Distribution may create a missing Target container and missing parents with ordinary user-directory permissions subject to `umask`; existing permissions are never changed. Skill Manager never deletes the Target container, including after its final Managed Link is removed.

Before inspection becomes mutation, Distribution resolves every existing path prefix again and verifies that:

- the physical Target identity still matches registration;
- Target and Skill Store are neither equal nor ancestors of each other;
- the filesystem root is not a Target;
- an existing container is a directory;
- the operation remains within the registered container.

A symlink introduced into a prefix after registration that redirects the Target to another physical location produces `target_redirected`, even when the new location appears safe. Migration or re-registration must be explicit. Mutation uses opened directory handles and no-follow operations for final slug entries to limit check-to-use races. Permission failures, read-only filesystems, or a path changing during mutation fail safely rather than degrading to less strict behavior.

### Filesystem inspection and Distribution Status

Inspection only touches desired slugs and paths present in the Managed Link ledger; it never enumerates or cleans unrelated Target content. It uses `lstat` on each final entry, records a symlink's raw target and physical resolution separately, and verifies destination existence, Store identity, basic Skill validity, and Target–Store safety.

Each Target–Skill relation has two orthogonal dimensions. Desired state is:

- `present` — selected by at least one Assignment;
- `absent` — no longer selected, but a Managed Link record still needs reconciliation.

Observed state is:

- `linked` — the symlink matches its ownership record and resolves to the valid expected Store Skill;
- `missing` — no entry exists at the slug path;
- `conflict` — an entry exists but cannot be proven to be the expected Managed Link;
- `broken_link` — the symlink still matches its ownership record but its destination is missing, invalid, or unsafe.

These combinations drive reconciliation:

- `present + linked`: satisfied;
- `present + missing`: create;
- `present + conflict`: block pending manual repair or eligible adoption;
- `present + broken_link`: block until the Store or safety problem is repaired;
- `absent + linked/broken_link`: safely remove the proven Managed Link;
- `absent + missing`: clear the stale ownership record;
- `absent + conflict`: leave the filesystem entry, relinquish the invalid ownership record, and report `ownership_lost`.

A missing Target container is a normal Target-level `missing` state and makes desired items `missing`; Distribution may create it. An incomplete inspection caused by permissions, I/O, or races installs no partial observation. It retains the previous observation, records the failure, and marks that observation stale.

Fresh inspection runs for an explicit status refresh, a WebUI refresh, and every reconciliation. Ordinary listings may return the latest stored observation with `last_inspected_at`. Without a watcher, the timestamp says when the state was observed rather than claiming it remains continuously current.

### Reconciliation behavior and outcomes

A reconciliation first completes the Target-level safety gate and one coherent inspection. A redirected, invalid, or unsafely opened Target receives no mutations. It then reconciles independent Skill entries, creating missing desired links before removing links that are no longer desired. One item's conflict, broken link, or I/O failure does not prevent other entries from completing safely.

Creation first records a random staging directory name, creates the symlink inside that fresh 0700 directory, and samples its identity through the pinned directory handle. It durably records that identity before publishing the symlink using an atomic no-overwrite rename. The final path is only verified against the staged identity; it never supplies a replacement identity. A path that appears concurrently becomes a conflict. This supersedes direct creation at the final slug: ticket 21 demonstrated that sampling that visible path after creation could certify an external replacement. Removal immediately rechecks the entry and unlinks only a symlink that still matches its ownership record. An explicit Store migration may update a proven Managed Link by removing the verified old link and creating the new link without overwrite; failure attempts to restore the old link only if the path remains empty, and never overwrites a concurrently created entry.

Target-level outcomes are:

- `succeeded` — every entry is satisfied or safely reconciled;
- `partial` — at least one entry is satisfied or reconciled while another is blocked or fails;
- `blocked` — inspection succeeds but safety or content conflicts permit no required progress;
- `failed` — Target-level inspection or operational failure prevents a trustworthy reconciliation.

Per-item outcomes are `no_op`, `created`, `removed`, `adopted`, `blocked_conflict`, `blocked_broken`, `ownership_lost`, and `failed`. The latest outcome, item results, timestamps, and errors are persisted; responses include the complete item summary.

### Conflict handling

Conflict details report the existing node type, raw and resolved symlink targets where applicable, the expected Store path, and whether prior ownership was lost. Skill Manager never offers Force, Replace, automatic backup, automatic move, or automatic deletion for unmanaged content.

Only an eligible exact-target symlink can be Adopted. Every other conflict requires the user to move or remove the item outside Skill Manager and retry, or remove the Assignment that makes the Skill desired. A previously managed path replaced by the user is treated by the same rule. Once the Skill is absent from desired state, the replacement is left untouched and the invalid ownership claim is discarded with an `ownership_lost` result.

### Assignment, Target, and Skill cleanup

Adding or removing Assignments and changing Group membership only changes desired state. The next explicit Distribution creates newly desired links and removes no-longer-desired links that remain provably managed.

Deleting a Target registration previews and confirms cleanup, removes every verifiable Managed Link, and retains the Target record if any safe deletion fails so the operation can be retried. Once cleanup succeeds, the registration is removed while its container remains.

Deleting a referenced Skill is blocked by default. Explicit deletion first removes related Assignments and then removes every verifiable Managed Link across Targets. Any cleanup failure retains the Store Skill. Paths whose ownership was already lost are warned about and left untouched. MVP has no ordinary shortcut that drops management records while silently leaving Managed Links behind.

### Concurrency, crash recovery, and retry

Reconciliation takes a shared Store lock followed by an exclusive lock for its Target. Store mutation uses the existing exclusive Store lock. Operations spanning Targets acquire Target locks in stable Target-ID order, allowing different Targets to reconcile concurrently without deadlocking global Skill cleanup. Lock files live in Skill Manager state rather than in Targets.

Before each link mutation, SQLite records a durable intent containing the action, path, expected raw target, and filesystem precondition. The filesystem operation then runs and the Managed Link ledger and result finalize afterward. Recovery handles unfinished intents deterministically:

- pending create first cleans only staged content matching its persisted identity and removes an empty staging directory; absent staging needs no cleanup;
- pending create with non-empty unproven, replaced, or unreadable staging preserves the content and intent and blocks with `recovery_failed`; it does not guess ownership or publish staged content;
- after staging cleanup, pending create plus a missing final path clears the intent and remains `missing`;
- after staging cleanup, pending create plus a final symlink matching its persisted raw target and physical identity completes ownership registration;
- after staging cleanup, pending create plus any other final entry preserves it as `conflict`; legacy intents without identity cannot establish ownership;
- pending remove plus a missing path completes ledger cleanup;
- pending remove plus the unchanged Managed Link safely retries removal;
- pending remove plus a changed entry leaves it untouched and records `ownership_lost`.

All creation is no-overwrite and every removal revalidates its precondition. External processes can still mutate a Target; Skill Manager guarantees detection and non-overwrite of unknown content rather than pretending to exclude external writers.

The private staging/isolation directories reduce races at public slug paths; they are not a security boundary against another process running as the same UID. Such a process can discover and modify a 0700 directory. Portable Unix operations provide neither atomic symlink-creation-with-an-inode-handle nor unlink-by-inode; paired observations and pinned handles do not promise exclusion of a malicious same-UID writer.

Reconciliation is idempotent and safe to rerun from fresh inspection. It performs no hidden, scheduled, background, or exponential retries. Users explicitly retry after correcting network-independent filesystem, permission, lock, Store, or conflict conditions.

# Batch Git tree listing and blob fetches

Type: task
Status: resolved
Blocked by: 03

## Question

During one observation, list the commit tree once and fetch every needed blob in one `cat-file --batch`, then validate Skills in memory. During materialize, `ls-tree` only the Skill path rather than the whole Source tree.

## Comments

- One Skill's invalid tree remains an Issue; it must not fail sibling Skills.
- Do not keep introducing per-Skill git processes for blob reads.

## Answer

`recordSkills` issues one `fetchTreeBlobs` for the whole observation (SKILL.md only in listing mode; every Skill blob in full mode). Invalid trees become Issues and do not fail siblings. Materialize `ls-tree`s `gitSkillTreePath(subpath, relativeDir)` only.

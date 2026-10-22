CREATE INDEX finding_target_subject_property_prefix
 ON finding(target_id,subject text_pattern_ops,property text_pattern_ops,id);
CREATE INDEX finding_revision_finding_basis
 ON finding_revision(finding_id,basis);
CREATE INDEX observation_target_source_member
 ON observation(target_id,(document #>> '{source,member}'));
CREATE INDEX observation_target_subject_prefix
 ON observation(target_id,(document->>'subject') text_pattern_ops);
CREATE INDEX observation_target_property_prefix
 ON observation(target_id,(document->>'property') text_pattern_ops);

---- create above / drop below ----
DROP INDEX observation_target_property_prefix;
DROP INDEX observation_target_subject_prefix;
DROP INDEX observation_target_source_member;
DROP INDEX finding_revision_finding_basis;
DROP INDEX finding_target_subject_property_prefix;

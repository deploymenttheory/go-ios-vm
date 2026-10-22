ALTER TABLE analysis_run ADD COLUMN input_sha256 text;
ALTER TABLE analysis_run ADD COLUMN input_size bigint;
ALTER TABLE analysis_run ADD COLUMN extractor text NOT NULL DEFAULT '';
ALTER TABLE analysis_run ADD COLUMN planned_components jsonb NOT NULL DEFAULT '[]';
UPDATE analysis_run r SET input_sha256=a.sha256,input_size=a.size
 FROM firmware_target t JOIN artifact a ON a.id=t.artifact_id WHERE t.id=r.target_id;
ALTER TABLE analysis_run ALTER COLUMN input_sha256 SET NOT NULL;
ALTER TABLE analysis_run ALTER COLUMN input_size SET NOT NULL;
ALTER TABLE analysis_run ADD CONSTRAINT analysis_run_input_size_nonnegative CHECK(input_size>=0);
CREATE TABLE run_batch (
 run_id text NOT NULL REFERENCES analysis_run(id),
 component text NOT NULL,
 ordinal integer NOT NULL CHECK(ordinal>=0),
 digest text NOT NULL CHECK(length(digest)=64),
 PRIMARY KEY(run_id,component,ordinal)
);
CREATE TABLE run_checkpoint (
 run_id text NOT NULL REFERENCES analysis_run(id),
 component text NOT NULL,
 state text NOT NULL CHECK(state IN ('complete','partial','unexamined')),
 reason text NOT NULL DEFAULT '',
 PRIMARY KEY(run_id,component)
);
CREATE TABLE run_coverage (
 run_id text NOT NULL REFERENCES analysis_run(id),
 component text NOT NULL,
 state text NOT NULL CHECK(state IN ('complete','partial','unexamined')),
 reason text NOT NULL DEFAULT '',
 PRIMARY KEY(run_id,component)
);
INSERT INTO run_coverage(run_id,component,state,reason)
 SELECT r.id,c.value->>'component',c.value->>'state',COALESCE(c.value->>'reason','')
 FROM analysis_run r CROSS JOIN LATERAL jsonb_array_elements(r.coverage) c
 WHERE r.state IN ('complete','partial');

---- create above / drop below ----
DROP TABLE run_coverage,run_checkpoint,run_batch;
ALTER TABLE analysis_run DROP COLUMN planned_components, DROP COLUMN extractor,
 DROP COLUMN input_size, DROP COLUMN input_sha256;

CREATE TABLE artifact (
 id text PRIMARY KEY, sha256 text NOT NULL UNIQUE CHECK(length(sha256)=64), size bigint NOT NULL CHECK(size>=0)
);
CREATE TABLE artifact_location (artifact_id text REFERENCES artifact(id), path text NOT NULL, PRIMARY KEY(artifact_id,path));
CREATE TABLE firmware_target (id text PRIMARY KEY, artifact_id text NOT NULL REFERENCES artifact(id), identity_index integer NOT NULL, document jsonb NOT NULL, UNIQUE(artifact_id,identity_index));
CREATE TABLE analysis_run (id text PRIMARY KEY, target_id text REFERENCES firmware_target(id), state text NOT NULL CHECK(state IN ('running','complete','partial','failed','cancelled')), started timestamptz NOT NULL, finished timestamptz, coverage jsonb NOT NULL DEFAULT '[]');
CREATE TABLE evidence_blob (id text PRIMARY KEY CHECK(length(id)=64), size bigint NOT NULL CHECK(size>=0), complete boolean NOT NULL DEFAULT false);
CREATE TABLE evidence_chunk (id text PRIMARY KEY, data bytea NOT NULL CHECK(octet_length(data)<=1048576));
CREATE TABLE blob_chunk (blob_id text REFERENCES evidence_blob(id), ordinal integer NOT NULL CHECK(ordinal>=0), chunk_id text NOT NULL REFERENCES evidence_chunk(id), PRIMARY KEY(blob_id,ordinal));
CREATE TABLE observation (id text PRIMARY KEY, target_id text NOT NULL REFERENCES firmware_target(id), evidence_id text NOT NULL REFERENCES evidence_blob(id), document jsonb NOT NULL);
CREATE TABLE run_observation (run_id text REFERENCES analysis_run(id), observation_id text REFERENCES observation(id), PRIMARY KEY(run_id,observation_id));
CREATE TABLE finding_revision (id text PRIMARY KEY, observation_id text NOT NULL REFERENCES observation(id), basis text NOT NULL CHECK(basis IN ('observed','inferred','unknown')), assessment text NOT NULL DEFAULT 'unreviewed' CHECK(assessment IN ('unreviewed','accepted','rejected','disputed')), supersedes text REFERENCES finding_revision(id), rationale text NOT NULL);
CREATE TABLE acquisition (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, source text NOT NULL, requested jsonb NOT NULL, status text NOT NULL, evidence_id text NOT NULL REFERENCES evidence_blob(id), created timestamptz NOT NULL DEFAULT now());
CREATE INDEX observation_target ON observation(target_id);
CREATE INDEX run_target ON analysis_run(target_id,started DESC);
CREATE INDEX observation_subject ON observation((document->>'subject'),(document->>'property'));
---- create above / drop below ----
DROP TABLE acquisition, finding_revision, run_observation, observation, blob_chunk, evidence_chunk, evidence_blob, analysis_run, firmware_target, artifact_location, artifact;

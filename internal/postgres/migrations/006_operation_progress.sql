CREATE TABLE operation_progress (
 id text PRIMARY KEY,
 kind text NOT NULL CHECK(kind IN ('acquisition','inspection')),
 run_id text UNIQUE REFERENCES analysis_run(id),
 request jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(octet_length(request::text)<=1024),
 input_sha256 text CHECK(input_sha256 IS NULL OR length(input_sha256)=64),
 input_size bigint CHECK(input_size IS NULL OR input_size>=0),
 source_fingerprint text CHECK(source_fingerprint IS NULL OR length(source_fingerprint)=64),
 phase text NOT NULL CHECK(length(phase) BETWEEN 1 AND 64),
 state text NOT NULL CHECK(state IN ('running','interrupted','complete','partial','failed','cancelled')),
 component text NOT NULL DEFAULT '' CHECK(length(component)<=128),
 completed_bytes bigint NOT NULL DEFAULT 0 CHECK(completed_bytes>=0),
 verified_bytes bigint NOT NULL DEFAULT 0 CHECK(verified_bytes>=0),
 total_bytes bigint CHECK(total_bytes IS NULL OR total_bytes>=0),
 completed_units bigint NOT NULL DEFAULT 0 CHECK(completed_units>=0),
 total_units bigint CHECK(total_units IS NULL OR total_units>=0),
 published_batches bigint NOT NULL DEFAULT 0 CHECK(published_batches>=0),
 diagnostic_code text NOT NULL DEFAULT '' CHECK(diagnostic_code ~ '^[a-z_]{0,64}$'),
 sequence bigint NOT NULL DEFAULT 0 CHECK(sequence>=0),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 started timestamptz NOT NULL DEFAULT now(),
 updated timestamptz NOT NULL DEFAULT now(),
 finished timestamptz,
 CHECK((kind='inspection' AND run_id=id AND input_sha256 IS NOT NULL AND input_size IS NOT NULL)
    OR (kind='acquisition' AND run_id IS NULL)),
 CHECK((state='running' AND finished IS NULL) OR (state<>'running' AND finished IS NOT NULL)),
 CHECK(total_bytes IS NULL OR (completed_bytes<=total_bytes AND verified_bytes<=total_bytes)),
 CHECK(total_units IS NULL OR completed_units<=total_units)
);
CREATE INDEX operation_progress_recent ON operation_progress(updated DESC,id);
CREATE TABLE operation_progress_event (
 operation_id text NOT NULL REFERENCES operation_progress(id) ON DELETE CASCADE,
 sequence bigint NOT NULL CHECK(sequence>0),
 phase text NOT NULL CHECK(length(phase) BETWEEN 1 AND 64),
 state text NOT NULL CHECK(state IN ('running','interrupted','complete','partial','failed','cancelled')),
 component text NOT NULL DEFAULT '' CHECK(length(component)<=128),
 diagnostic_code text NOT NULL DEFAULT '' CHECK(diagnostic_code ~ '^[a-z_]{0,64}$'),
 created timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(operation_id,sequence)
);
ALTER TABLE acquisition ADD COLUMN attempt_id text REFERENCES operation_progress(id);
CREATE INDEX acquisition_attempt ON acquisition(attempt_id,created,id);

INSERT INTO operation_progress
 (id,kind,run_id,input_sha256,input_size,phase,state,completed_units,
  total_units,published_batches,sequence,started,updated,finished)
 SELECT r.id,'inspection',r.id,r.input_sha256,r.input_size,'historical',r.state,
  (SELECT count(*) FROM run_checkpoint c WHERE c.run_id=r.id),
  CASE WHEN jsonb_array_length(r.planned_components)>0
   THEN jsonb_array_length(r.planned_components) ELSE NULL END,
  (SELECT count(*) FROM run_batch b WHERE b.run_id=r.id),
  1,r.started,COALESCE(r.finished,now()),
  CASE WHEN r.state='running' THEN NULL ELSE COALESCE(r.finished,now()) END
 FROM analysis_run r;
INSERT INTO operation_progress_event(operation_id,sequence,phase,state)
 SELECT id,1,phase,state FROM operation_progress;

---- create above / drop below ----
ALTER TABLE acquisition DROP COLUMN attempt_id;
DROP TABLE operation_progress_event, operation_progress;

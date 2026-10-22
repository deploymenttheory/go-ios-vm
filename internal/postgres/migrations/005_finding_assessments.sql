CREATE UNIQUE INDEX finding_revision_assessment_owner ON finding_revision(id,finding_id,basis);
CREATE TABLE finding_assessment_event (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 finding_id text NOT NULL,
 revision_id text NOT NULL,
 basis text NOT NULL,
 state text NOT NULL CHECK(state IN ('accepted','rejected','disputed','needs_review')),
 reviewer text NOT NULL CHECK(length(trim(reviewer))>0),
 reason text NOT NULL CHECK(length(trim(reason))>0),
 previous_id bigint REFERENCES finding_assessment_event(id),
 cause_revision_id text REFERENCES finding_revision(id),
 created timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(revision_id,finding_id,basis)
  REFERENCES finding_revision(id,finding_id,basis),
 CHECK((state='needs_review' AND reviewer='system') OR
       (state<>'needs_review' AND reviewer<>'system' AND cause_revision_id IS NULL))
);
CREATE UNIQUE INDEX finding_assessment_first ON finding_assessment_event(revision_id)
 WHERE previous_id IS NULL;
CREATE UNIQUE INDEX finding_assessment_successor ON finding_assessment_event(revision_id,previous_id)
 WHERE previous_id IS NOT NULL;
CREATE INDEX finding_assessment_history ON finding_assessment_event(revision_id,id DESC);

INSERT INTO finding_assessment_event(finding_id,revision_id,basis,state,reviewer,reason)
 SELECT finding_id,id,basis,assessment,'legacy',
  'Migrated from the prior assessment column; reviewer reason was not retained'
 FROM finding_revision WHERE assessment<>'unreviewed';
INSERT INTO finding_assessment_event(finding_id,revision_id,basis,state,reviewer,reason,previous_id)
 SELECT e.finding_id,e.revision_id,e.basis,'needs_review','system',
  'Legacy assessment lacks its original reviewer reason; review this revision again',
  e.id
 FROM finding_assessment_event e WHERE e.reviewer='legacy';
ALTER TABLE finding_revision DROP COLUMN assessment;

CREATE FUNCTION guard_finding_assessment() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE latest bigint;
DECLARE owner_target text;
DECLARE cause_target text;
BEGIN
 IF TG_OP<>'INSERT' THEN
  RAISE EXCEPTION 'assessment history is immutable';
 END IF;
 PERFORM 1 FROM finding_revision WHERE id=NEW.revision_id FOR UPDATE;
 SELECT id INTO latest FROM finding_assessment_event
  WHERE revision_id=NEW.revision_id ORDER BY id DESC LIMIT 1;
 IF NEW.previous_id IS DISTINCT FROM latest THEN
  RAISE EXCEPTION 'stale assessment predecessor';
 END IF;
 IF NEW.cause_revision_id IS NOT NULL THEN
  SELECT f.target_id INTO owner_target FROM finding_revision r
   JOIN finding f ON f.id=r.finding_id WHERE r.id=NEW.revision_id;
  SELECT f.target_id INTO cause_target FROM finding_revision r
   JOIN finding f ON f.id=r.finding_id WHERE r.id=NEW.cause_revision_id;
  IF cause_target IS DISTINCT FROM owner_target OR
     NEW.cause_revision_id=NEW.revision_id THEN
   RAISE EXCEPTION 'assessment invalidation must name another revision of the same target';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER finding_assessment_guard BEFORE INSERT OR UPDATE OR DELETE
 ON finding_assessment_event FOR EACH ROW EXECUTE FUNCTION guard_finding_assessment();

---- create above / drop below ----
DROP TRIGGER finding_assessment_guard ON finding_assessment_event;
DROP FUNCTION guard_finding_assessment();
ALTER TABLE finding_revision ADD COLUMN assessment text NOT NULL DEFAULT 'unreviewed'
 CHECK(assessment IN ('unreviewed','accepted','rejected','disputed'));
UPDATE finding_revision r SET assessment=e.state FROM (
 SELECT DISTINCT ON (revision_id) revision_id,state FROM finding_assessment_event
 ORDER BY revision_id,id DESC
) e WHERE e.revision_id=r.id AND e.state IN ('accepted','rejected','disputed');
DROP TABLE finding_assessment_event;
DROP INDEX finding_revision_assessment_owner;

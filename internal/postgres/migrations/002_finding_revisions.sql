CREATE TABLE finding (
 id text PRIMARY KEY DEFAULT gen_random_uuid()::text,
 target_id text NOT NULL REFERENCES firmware_target(id),
 subject text NOT NULL CHECK(subject <> ''),
 property text NOT NULL CHECK(property <> ''),
 UNIQUE(target_id,subject,property),
 UNIQUE(id,target_id)
);
INSERT INTO finding(target_id,subject,property)
 SELECT DISTINCT target_id,document->>'subject',document->>'property' FROM observation;

ALTER TABLE finding_revision ADD COLUMN finding_id text;
ALTER TABLE finding_revision ADD COLUMN presence text;
ALTER TABLE finding_revision ADD COLUMN value jsonb;
ALTER TABLE finding_revision ADD COLUMN author text NOT NULL DEFAULT 'inspection';
ALTER TABLE finding_revision ADD COLUMN created timestamptz NOT NULL DEFAULT now();
UPDATE finding_revision r SET
 finding_id=f.id,
 presence=o.document->>'presence',
 value=o.document->'value'
 FROM observation o JOIN finding f ON f.target_id=o.target_id
  AND f.subject=o.document->>'subject' AND f.property=o.document->>'property'
 WHERE r.observation_id=o.id;
ALTER TABLE finding_revision ALTER COLUMN finding_id SET NOT NULL;
ALTER TABLE finding_revision ALTER COLUMN observation_id DROP NOT NULL;
ALTER TABLE finding_revision ADD CONSTRAINT finding_revision_finding_fk FOREIGN KEY(finding_id) REFERENCES finding(id);
ALTER TABLE finding_revision ADD CONSTRAINT finding_revision_presence_check CHECK(presence IN ('present','absent','unexamined'));
ALTER TABLE finding_revision ADD CONSTRAINT finding_revision_basis_observation_check CHECK(
 (basis='observed' AND observation_id IS NOT NULL) OR
 (basis IN ('inferred','unknown') AND observation_id IS NULL)
);
ALTER TABLE finding_revision ADD CONSTRAINT finding_revision_value_check CHECK(
 (basis='unknown' AND presence='unexamined' AND value IS NULL) OR
 (basis<>'unknown' AND ((presence='present' AND value IS NOT NULL) OR (presence<>'present' AND value IS NULL)))
);
ALTER TABLE finding_revision ADD CONSTRAINT finding_revision_rationale_check CHECK(length(trim(rationale))>0);
CREATE UNIQUE INDEX finding_revision_finding_id_id ON finding_revision(finding_id,id);
ALTER TABLE finding_revision ADD CONSTRAINT finding_revision_same_finding_supersedes
 FOREIGN KEY(finding_id,supersedes) REFERENCES finding_revision(finding_id,id);

CREATE TABLE finding_basis (
 revision_id text NOT NULL REFERENCES finding_revision(id),
 role text NOT NULL CHECK(role IN ('supporting','contradicting')),
 observation_id text REFERENCES observation(id),
 reference_revision_id text REFERENCES finding_revision(id),
 CHECK((observation_id IS NULL) <> (reference_revision_id IS NULL)),
 CHECK(reference_revision_id IS DISTINCT FROM revision_id),
 UNIQUE(revision_id,role,observation_id),
 UNIQUE(revision_id,role,reference_revision_id)
);
INSERT INTO finding_basis(revision_id,role,observation_id)
 SELECT id,'supporting',observation_id FROM finding_revision WHERE observation_id IS NOT NULL;

CREATE FUNCTION check_finding_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE question finding%ROWTYPE; seen observation%ROWTYPE;
BEGIN
 IF TG_OP='UPDATE' AND ROW(NEW.id,NEW.finding_id,NEW.observation_id,NEW.basis,NEW.supersedes,
  NEW.rationale,NEW.presence,NEW.value,NEW.author,NEW.created) IS DISTINCT FROM
  ROW(OLD.id,OLD.finding_id,OLD.observation_id,OLD.basis,OLD.supersedes,
  OLD.rationale,OLD.presence,OLD.value,OLD.author,OLD.created) THEN
  RAISE EXCEPTION 'finding revisions are immutable; create a linked revision';
 END IF;
 SELECT * INTO question FROM finding WHERE id=NEW.finding_id;
 IF NEW.basis='observed' THEN
  SELECT * INTO seen FROM observation WHERE id=NEW.observation_id;
  IF seen.id IS NULL OR seen.target_id<>question.target_id OR
   seen.document->>'subject'<>question.subject OR seen.document->>'property'<>question.property OR
   NOT EXISTS(SELECT 1 FROM evidence_blob WHERE id=seen.evidence_id AND complete) OR
   NOT EXISTS(SELECT 1 FROM run_observation WHERE observation_id=seen.id) THEN
   RAISE EXCEPTION 'observed revision requires a published matching observation';
  END IF;
  NEW.presence:=seen.document->>'presence';
  NEW.value:=CASE WHEN NEW.presence='present' THEN seen.document->'value' ELSE NULL END;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER finding_revision_validate BEFORE INSERT OR UPDATE ON finding_revision
 FOR EACH ROW EXECUTE FUNCTION check_finding_revision();

CREATE FUNCTION check_finding_basis() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE owner_target text; ref_target text;
BEGIN
 SELECT f.target_id INTO owner_target FROM finding_revision r JOIN finding f ON f.id=r.finding_id WHERE r.id=NEW.revision_id;
 IF NEW.observation_id IS NOT NULL THEN
  SELECT o.target_id INTO ref_target FROM observation o JOIN evidence_blob b ON b.id=o.evidence_id
   WHERE o.id=NEW.observation_id AND b.complete;
 ELSE
  SELECT f.target_id INTO ref_target FROM finding_revision r JOIN finding f ON f.id=r.finding_id
   WHERE r.id=NEW.reference_revision_id;
 END IF;
 IF owner_target IS NULL OR ref_target IS NULL OR owner_target<>ref_target THEN
  RAISE EXCEPTION 'finding basis must reference published evidence from the same target';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER finding_basis_validate BEFORE INSERT OR UPDATE ON finding_basis
 FOR EACH ROW EXECUTE FUNCTION check_finding_basis();

CREATE FUNCTION preserve_finding_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'finding evidence is immutable; create a linked revision';
END $$;
CREATE TRIGGER observation_immutable BEFORE UPDATE OR DELETE ON observation
 FOR EACH ROW EXECUTE FUNCTION preserve_finding_evidence();
CREATE TRIGGER finding_basis_immutable BEFORE UPDATE OR DELETE ON finding_basis
 FOR EACH ROW EXECUTE FUNCTION preserve_finding_evidence();
CREATE TRIGGER finding_identity_immutable BEFORE UPDATE OR DELETE ON finding
 FOR EACH ROW EXECUTE FUNCTION preserve_finding_evidence();

---- create above / drop below ----
DROP TRIGGER finding_identity_immutable ON finding;
DROP TRIGGER finding_basis_immutable ON finding_basis;
DROP TRIGGER observation_immutable ON observation;
DROP FUNCTION preserve_finding_evidence();
DROP TRIGGER finding_basis_validate ON finding_basis;
DROP FUNCTION check_finding_basis();
DROP TRIGGER finding_revision_validate ON finding_revision;
DROP FUNCTION check_finding_revision();
DROP TABLE finding_basis;
ALTER TABLE finding_revision DROP CONSTRAINT finding_revision_same_finding_supersedes;
DROP INDEX finding_revision_finding_id_id;
ALTER TABLE finding_revision DROP CONSTRAINT finding_revision_rationale_check;
ALTER TABLE finding_revision DROP CONSTRAINT finding_revision_value_check;
ALTER TABLE finding_revision DROP CONSTRAINT finding_revision_basis_observation_check;
ALTER TABLE finding_revision DROP CONSTRAINT finding_revision_presence_check;
ALTER TABLE finding_revision DROP CONSTRAINT finding_revision_finding_fk;
ALTER TABLE finding_revision DROP COLUMN created, DROP COLUMN author, DROP COLUMN value, DROP COLUMN presence, DROP COLUMN finding_id;
ALTER TABLE finding_revision ALTER COLUMN observation_id SET NOT NULL;
DROP TABLE finding;

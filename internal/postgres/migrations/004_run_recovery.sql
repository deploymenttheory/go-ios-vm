ALTER TABLE analysis_run DROP CONSTRAINT analysis_run_state_check;
ALTER TABLE analysis_run ADD CONSTRAINT analysis_run_state_check
 CHECK(state IN ('running','interrupted','complete','partial','failed','cancelled'));
ALTER TABLE analysis_run ADD COLUMN lease_token text;
ALTER TABLE analysis_run ADD COLUMN lease_expires timestamptz;
UPDATE analysis_run SET lease_expires=now() WHERE state='running';
ALTER TABLE analysis_run ADD CONSTRAINT analysis_run_finished_matches_state CHECK (
 (state IN ('running','interrupted') AND finished IS NULL) OR
 (state IN ('complete','partial','failed','cancelled') AND finished IS NOT NULL)
);
CREATE INDEX analysis_run_expired_lease ON analysis_run(lease_expires)
 WHERE state='running';

CREATE FUNCTION guard_run_terminal() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE planned jsonb;
DECLARE ledger_count integer;
DECLARE incomplete_count integer;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF OLD.state IN ('complete','partial') AND
     (NEW.state IS DISTINCT FROM OLD.state OR NEW.coverage IS DISTINCT FROM OLD.coverage OR
      NEW.finished IS DISTINCT FROM OLD.finished) THEN
   RAISE EXCEPTION 'terminal run cannot change';
  END IF;
  IF OLD.state IN ('failed','cancelled') AND NEW.state NOT IN (OLD.state,'running') THEN
   RAISE EXCEPTION 'failed or cancelled run can only be explicitly resumed';
  END IF;
  IF NEW.state IN ('complete','partial') AND OLD.state IS DISTINCT FROM NEW.state AND OLD.state<>'running' THEN
   RAISE EXCEPTION 'only a running run can finalize';
  END IF;
  IF NEW.state IN ('complete','partial') AND OLD.state IS DISTINCT FROM NEW.state THEN
   planned := NEW.planned_components;
   IF jsonb_typeof(planned)<>'array' THEN planned:='[]'::jsonb; END IF;
   SELECT count(*),count(*) FILTER (WHERE state<>'complete')
    INTO ledger_count,incomplete_count FROM run_coverage WHERE run_id=NEW.id;
   IF ledger_count=0 OR
      (jsonb_array_length(planned)>0 AND
       (ledger_count<>jsonb_array_length(planned) OR EXISTS (
         SELECT 1 FROM run_coverage c WHERE c.run_id=NEW.id AND
          NOT planned ? c.component))) OR
      (NEW.state='complete' AND incomplete_count>0) OR
      (NEW.state='partial' AND incomplete_count=0) THEN
    RAISE EXCEPTION 'terminal coverage ledger does not match run state and plan';
   END IF;
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guard_run_terminal BEFORE UPDATE ON analysis_run
 FOR EACH ROW EXECUTE FUNCTION guard_run_terminal();

CREATE FUNCTION guard_run_coverage() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE current_state text;
DECLARE key text;
BEGIN
 IF TG_OP='INSERT' THEN key:=NEW.run_id; ELSE key:=OLD.run_id; END IF;
 SELECT state INTO current_state FROM analysis_run WHERE id=key FOR UPDATE;
 IF current_state<>'running' THEN
  RAISE EXCEPTION 'coverage can only be published for a running run';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guard_run_coverage BEFORE INSERT OR UPDATE OR DELETE ON run_coverage
 FOR EACH ROW EXECUTE FUNCTION guard_run_coverage();

---- create above / drop below ----
DROP TRIGGER guard_run_coverage ON run_coverage;
DROP FUNCTION guard_run_coverage();
DROP TRIGGER guard_run_terminal ON analysis_run;
DROP FUNCTION guard_run_terminal();
DROP INDEX analysis_run_expired_lease;
ALTER TABLE analysis_run DROP CONSTRAINT analysis_run_finished_matches_state;
ALTER TABLE analysis_run DROP COLUMN lease_expires, DROP COLUMN lease_token;
ALTER TABLE analysis_run DROP CONSTRAINT analysis_run_state_check;
ALTER TABLE analysis_run ADD CONSTRAINT analysis_run_state_check
 CHECK(state IN ('running','complete','partial','failed','cancelled'));

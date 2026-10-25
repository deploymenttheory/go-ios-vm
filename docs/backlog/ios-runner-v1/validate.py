#!/usr/bin/env python3
"""Validate the local approval preview without accessing or changing GitHub."""
import hashlib
import json
from pathlib import Path
import re


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


root = Path(__file__).resolve().parent
docs = root.parents[1]
backlog = json.loads((root / 'backlog.json').read_text())
inventory = json.loads((root / 'remote-inventory.json').read_text())
preview = (docs / 'github-backlog.md').read_text()
assert digest(docs / 'implementation-plan.md') == backlog['source']['sha256']
assert backlog['status'] == 'draft-awaiting-approval'
assert backlog['approval'] is None
issues = {issue['id']: issue for issue in backlog['issues']}
assert len(issues) == len(backlog['issues']), 'Duplicate stable identity'
labels = {label['name'] for label in backlog['labels']}
gates = {gate['id']: position for position, gate in enumerate(backlog['milestones'])}
tasks = [issue for issue in issues.values() if issue['kind'] == 'task']
epics = [issue for issue in issues.values() if issue['kind'] == 'epic']
required = ['Outcome', 'Implementation', 'Acceptance criteria', 'Verification',
            'Dependencies', 'Applicable product rules', 'Planning',
            'Architecture decision alignment']
seen, active = set(), set()


def visit(identity):
    assert identity not in active, ('Cycle', identity)
    if identity in seen:
        return
    active.add(identity)
    for predecessor in issues[identity]['blocked_by']:
        assert predecessor in issues and issues[predecessor]['kind'] == 'task'
        assert gates[issues[predecessor]['milestone']] <= gates[issues[identity]['milestone']]
        visit(predecessor)
    active.remove(identity)
    seen.add(identity)


def ancestors(identity):
    result = set()
    for predecessor in issues[identity]['blocked_by']:
        result.add(predecessor)
        result.update(ancestors(predecessor))
    return result


for issue in issues.values():
    identity = issue['id']
    visit(identity)
    assert issue['milestone'] in gates
    assert set(issue['labels']) <= labels
    assert issue['body'].startswith(f'<!-- weave-backlog:{backlog["namespace"]}:v1:{identity} -->')
    assert issue['body'] in preview, ('Preview body differs', identity)
    assert preview.count(f'<a id="{identity.lower()}"></a>') == 1
    assert len(issue['blocked_by']) == len(set(issue['blocked_by']))
    for reference in re.findall(r'\[([A-Z]+(?:-[A-Z0-9]+)*)\]\(#[a-z0-9-]+\)', issue['body']):
        assert reference in issues, ('Unresolved draft identity', identity, reference)
    if issue['kind'] == 'task':
        assert re.findall(r'^## (.+)$', issue['body'], re.M) == required
        parent = issues[issue['parent']]
        assert parent['kind'] == 'epic'
        assert gates[issue['milestone']] <= gates[parent['milestone']]
        assert f'[{identity}](#{identity.lower()})' in parent['body']
        assert all(any(label.startswith(prefix) for label in issue['labels'])
                   for prefix in ['type:', 'priority:', 'area:', 'component:'])
        assert '- [ ] ' in issue['body']
    else:
        assert issue['parent'] is None
    for predecessor in issue['blocked_by']:
        others = set(issue['blocked_by']) - {predecessor}
        assert not any(predecessor in ancestors(other) for other in others), ('Redundant transitive blocker', identity, predecessor)

for requirement in backlog['requirements']:
    assert requirement['rationale']
    assert all(identity in issues for identity in requirement['tickets'])
assert all(any(issue['id'] in row['tickets'] for row in backlog['requirements']) for issue in tasks)
ready = sorted(issue['id'] for issue in tasks if not issue['blocked_by'])
assert ready == sorted(backlog['ready_after_publication'])
assert all(row['read_ok'] for row in inventory['relationships'])
old_labels = {label['name']: label for label in inventory['labels']}
for label in backlog['labels']:
    if label['action'] == 'reuse':
        assert label['color'] == old_labels[label['name']]['color']
        assert label['description'] == old_labels[label['name']]['description']
    else:
        assert label['name'] not in old_labels
assert not any(f'weave-backlog:{backlog["namespace"]}:v1:' in (row.get('body') or '') for row in inventory['issues'])
assert sorted(backlog['existing_work']['unchanged_issue_numbers']) == sorted(row['number'] for row in inventory['issues'])
assert not any(backlog['existing_work'][key] for key in ['updates', 'obsolete_proposals', 'conflicts'])
assert not backlog['publication']['journal'] and not backlog['publication']['issue_mapping']
assert all(gate['due_on'] is None for gate in backlog['milestones'])

print(json.dumps({
    'status': 'passed', 'repository': backlog['repository'],
    'namespace': backlog['namespace'],
    'source_plan_sha256': backlog['source']['sha256'],
    'preview_sha256': digest(docs / 'github-backlog.md'),
    'desired_state_sha256': digest(root / 'backlog.json'),
    'issues': len(issues), 'epics': len(epics), 'implementation_tasks': len(tasks),
    'native_parent_relationships': len(tasks),
    'native_blocker_relationships': sum(len(issue['blocked_by']) for issue in tasks),
    'new_labels': sum(label['action'] == 'create' for label in backlog['labels']),
    'reused_labels': sum(label['action'] == 'reuse' for label in backlog['labels']),
    'new_milestones': len(backlog['milestones']),
    'existing_issues_unchanged': len(inventory['issues']),
    'successful_native_relationship_inventory_reads': len(inventory['relationships']),
    'ready_after_publication': ready,
    'checks': ['source fingerprint', 'full-preview/body agreement', 'unique identities',
               'eight implementation sections', 'parent membership', 'acyclic blockers',
               'no redundant transitive edges', 'milestone ordering',
               'requirement coverage', 'label reuse without changes',
               'readiness', 'no existing namespace overlap',
               'empty publication journal/mapping'],
    'not_verified': ['GitHub write capability', 'application implementation',
                     'native VM execution', 'restore', 'graphical guest boot'],
}, indent=2))

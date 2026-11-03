const fs = require('node:fs');
const args = process.argv.slice(2);
const file = process.env.PHASE_E_FIXTURE_STATE;
// Native permission reads can run concurrently. Serialize the stand-in's
// read/modify/write transaction so it cannot truncate or lose test state.
const lock = file + '.lock';
const deadline = Date.now() + 5000;
while (true) {
  try { fs.mkdirSync(lock); break; }
  catch (error) {
    if (error.code !== 'EEXIST' || Date.now() >= deadline) throw error;
    Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 5);
  }
}
process.on('exit', () => fs.rmdirSync(lock));
const state = JSON.parse(fs.readFileSync(file, 'utf8'));
const prefix = '/repos/example/phase-e-fixture';
const raw = args.find(a => a.startsWith(prefix) || a.startsWith(prefix.slice(1))) || '';
const endpoint = (raw.startsWith('/') ? raw : '/' + raw).split('?')[0];
const methodIndex = args.indexOf('--method');
const method = methodIndex < 0 ? 'GET' : args[methodIndex + 1];
const output = (value) => {
  const index = args.indexOf('--jq');
  if (index < 0) return process.stdout.write(JSON.stringify(value));
  const { spawnSync } = require('node:child_process');
  const result = spawnSync('jq', ['-r', args[index + 1]], { input: JSON.stringify(value), encoding: 'utf8' });
  if (result.status !== 0) process.exit(result.status || 1);
  process.stdout.write(result.stdout);
};
const pages = (value) => output(args.includes('--slurp') ? [value] : value);
const save = () => fs.writeFileSync(file, JSON.stringify(state));
state.calls.push({ method, endpoint });
save();
const permissions = process.env.GH_TOKEN === 'synthetic-dispatch-token' ? state.dispatchPermissions : state.permissions;
const need = endpoint.includes('/actions/') ? 'actions'
  : endpoint.includes('/check-runs') ? 'checks'
  : endpoint.endsWith('/status') ? 'statuses'
  : endpoint.includes('/pulls/') ? 'pull-requests'
  : endpoint.includes('/issues/') ? 'issues' : null;
if (need && !['read', 'write'].includes(permissions[need])) {
  process.stderr.write('Resource not accessible by integration (HTTP 403)'); process.exit(1);
}
if (need && method !== 'GET' && permissions[need] !== 'write') {
  process.stderr.write('Resource not accessible by integration (HTTP 403)'); process.exit(1);
}
const field = (name) => args.find(a => a.startsWith(name + '='))?.slice(name.length + 1);
if (args[0] === 'pr' && args[1] === 'checks') {
  const checkState = state.checkState || 'SUCCESS';
  output([{name: 'PR Gate', state: checkState}]);
  if (checkState !== 'SUCCESS') process.exit(checkState === 'PENDING' ? 8 : 1);
}
else if (endpoint === prefix + '/actions/workflows/ground-control-phase-e.yml/dispatches' && method === 'POST') {
  state.dispatches.push(JSON.parse(fs.readFileSync(0, 'utf8'))); save(); output({});
}
else if (args[0] === 'release' && args[1] === 'download') {
  const asset = args[args.indexOf('--pattern') + 1];
  const destination = args[args.indexOf('--dir') + 1];
  fs.copyFileSync(state.asset, require('node:path').join(destination, asset));
}
else if (args[0] === 'repo' && args[1] === 'view') output({nameWithOwner:'example/phase-e-fixture'});
else if (args[0] === 'api' && (args.includes('user') || args.includes('/user'))) { process.stderr.write('Resource not accessible by integration (HTTP 403)'); process.exit(1); }
else if (endpoint.includes('/collaborators/') && endpoint.endsWith('/permission')) {
  const permission = endpoint.includes('/maintainer/') ? 'write' : 'none';
  output({permission});
} else if (endpoint === prefix + '/pulls/42') output(state.pr);
else if (endpoint === prefix + '/issues/1103/timeline') pages([{event:'cross-referenced',source:{issue:{number:42,pull_request:{},repository:{full_name:'example/phase-e-fixture'}}}}]);
else if (endpoint === prefix + '/issues/42/comments') pages(state.pointer);
else if (endpoint === prefix + '/issues/1103/comments' && method === 'GET') pages(state.comments);
else if (endpoint === prefix + '/issues/1103/comments' && method === 'POST') {
  const body=field('body'); if (typeof body !== 'string') throw new Error('missing body');
  const comment={id:1000+state.comments.length,body,user:{login:'github-actions[bot]',type:'Bot'},author_association:'NONE'};
  state.comments.push(comment); save(); output({...comment,html_url:'https://github.com/example/phase-e-fixture/issues/1103#issuecomment-'+comment.id});
} else if (endpoint === prefix + '/issues/1103' && method === 'GET') output({number:1103,title:'Delivery',body:'',state:state.closed?'closed':'open',labels:[]});
else if (endpoint === prefix + '/issues/1103' && method === 'PATCH' && field('state') === 'closed') {
  state.closed=true; state.closes.push(endpoint);save();output({number:1103,state:'closed'});
} else if (endpoint === prefix + '/issues/1103/labels/in-progress' && method === 'DELETE') output({});
else if (endpoint === prefix + '/actions/runs/7') output({path:'.github/workflows/ground-control-phase-e.yml',repository:{full_name:'example/phase-e-fixture'},pull_requests:[],event:state.event,display_title:'Ground Control Phase E for PR ' + state.provenancePr,status:'in_progress'});
else if (endpoint.includes('/protection/required_status_checks')) output({contexts:['tests']});
else if (endpoint.includes('/check-runs')) pages({check_runs:[{name:'tests',status:'completed',conclusion:'success'}]});
else if (endpoint.endsWith('/status')) output({statuses:[]});
else { process.stderr.write('Unexpected GitHub boundary: '+JSON.stringify(args)); process.exit(2); }

import { spawn } from 'child_process';

const server = spawn('node', ['packages/mcp-server/dist/index.js'], {
  env: { ...process.env, APERTURE_BASE_URL: 'http://localhost:8080' }
});

server.stderr.on('data', (data) => {
  console.error(`STDERR: ${data}`);
});

server.stdout.on('data', (data) => {
  console.log(`STDOUT: ${data}`);
});

const request = {
  jsonrpc: '2.0',
  id: 1,
  method: 'tools/list',
  params: {}
};

server.stdin.write(JSON.stringify(request) + '\n');

setTimeout(() => {
  server.kill();
  process.exit(0);
}, 5000);

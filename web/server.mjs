// 客户端 / 司机端页面的本地静态服务器：零依赖，只用 Node 内置模块。
//
// 它做两件事：
//   1. 托管 web/public 下的静态页面（乘客端 /、司机端 /driver/）；
//   2. 把 /api/v1 反向代理到 Gin 网关（默认 http://127.0.0.1:8080），
//      让页面与接口同源，浏览器不需要依赖网关的 CORS 配置。
//
// 用法：node server.mjs [--port 8081] [--api http://127.0.0.1:8080]
import http from 'node:http';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const publicDir = path.join(here, 'public');

/**
 * parseArgs 读取可选的监听端口与后端地址；端口优先取命令行，其次 PORT 环境变量，
 * 默认值让 `node server.mjs` 单独也能跑起来。
 */
function parseArgs(argv) {
  const options = { port: Number(process.env.PORT) || 8081, api: process.env.API_BASE || 'http://127.0.0.1:8080' };
  for (let i = 0; i < argv.length; i += 1) {
    if (argv[i] === '--port') options.port = Number(argv[i + 1]);
    if (argv[i] === '--api') options.api = argv[i + 1];
  }
  return options;
}

const options = parseArgs(process.argv.slice(2));
const apiBase = new URL(options.api);

/**
 * isPortFree 同时探测回环地址与通配地址：只有两者都能绑定，端口才算可用。
 * Windows 上两者可能给出不同结论，dev server 实际会绑定通配地址。
 */
function isPortFree(port) {
  return new Promise((resolve) => {
    const server = http.createServer();
    server.once('error', () => resolve(false));
    server.once('listening', () => {
      server.close(() => resolve(true));
    });
    server.listen(port, '127.0.0.1');
  });
}

/** resolvePort 从期望端口开始顺延，找到第一个空闲端口。 */
async function resolvePort(preferred) {
  for (let port = preferred; port < preferred + 20; port += 1) {
    if (await isPortFree(port)) return port;
  }
  throw new Error(`no free port in range ${preferred}..${preferred + 19}`);
}

const mimeTypes = {
  '.html': 'text/html; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
};

/** sendFile 输出静态文件；找不到时交给调用方回退到 index.html。 */
function sendFile(response, filePath) {
  fs.readFile(filePath, (error, content) => {
    if (error) {
      response.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
      response.end('not found');
      return;
    }
    // no-cache：静态资源每次做协商校验，避免改版后浏览器仍跑旧脚本（演示环境资源很小）。
    response.writeHead(200, {
      'Content-Type': mimeTypes[path.extname(filePath)] || 'application/octet-stream',
      'Cache-Control': 'no-cache',
    });
    response.end(content);
  });
}

/**
 * proxyToApi 把请求原样转发给 Gin 网关，并把状态码、响应头和响应体带回浏览器。
 * 网关已配置 CORS，但同源代理可以让页面不用关心预检与自定义响应头。
 */
function proxyToApi(request, response, pathWithQuery) {
  const target = new URL(pathWithQuery, apiBase);
  const headers = { ...request.headers };
  delete headers.host;
  delete headers['content-length'];

  const upstream = http.request(
    target,
    { method: request.method, headers },
    (upstreamResponse) => {
      response.writeHead(upstreamResponse.statusCode || 502, upstreamResponse.headers);
      upstreamResponse.pipe(response);
    },
  );
  upstream.on('error', (error) => {
    response.writeHead(502, { 'Content-Type': 'application/json; charset=utf-8' });
    response.end(JSON.stringify({ error: { code: 'bad_gateway', message: `gateway unreachable: ${error.message}` } }));
  });
  request.pipe(upstream);
}

const server = http.createServer((request, response) => {
  const url = new URL(request.url, `http://${request.headers.host || '127.0.0.1'}`);
  const pathname = decodeURIComponent(url.pathname);

  // 编排脚本用它判断页面服务是否就绪，不依赖后端接口是否可达。
  if (pathname === '/healthz') {
    response.writeHead(200, { 'Content-Type': 'application/json; charset=utf-8' });
    response.end(JSON.stringify({ status: 'ok', api: apiBase.origin, port: server.address()?.port ?? options.port }));
    return;
  }

  if (pathname.startsWith('/api/')) {
    proxyToApi(request, response, `${pathname}${url.search}`);
    return;
  }

  let relative = pathname === '/' ? 'index.html' : pathname.replace(/^\/+/, '');
  let filePath = path.join(publicDir, relative);
  // 目录请求回落到该目录下的 index.html，所以 /driver/ 能直接打开司机端。
  if (!path.extname(relative)) {
    filePath = path.join(filePath, 'index.html');
  }
  if (!filePath.startsWith(publicDir)) {
    response.writeHead(403, { 'Content-Type': 'text/plain; charset=utf-8' });
    response.end('forbidden');
    return;
  }
  sendFile(response, filePath);
});

const port = await resolvePort(options.port);
server.listen(port, '0.0.0.0', () => {
  console.log('');
  console.log('  花小猪打车 · 乘客/司机端页面已启动');
  console.log(`  乘客端  http://localhost:${port}/`);
  console.log(`  司机端  http://localhost:${port}/driver/`);
  console.log(`  接口    ${apiBase.origin}/api/v1 （同源代理 /api/v1）`);
  console.log('');
});

for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => {
    server.close(() => process.exit(0));
  });
}

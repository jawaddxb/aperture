/**
 * config.test.ts — Unit tests for config loading.
 */

import { loadConfig } from './config';

describe('loadConfig', () => {
  const originalEnv = process.env;

  beforeEach(() => {
    process.env = { ...originalEnv };
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  it('returns defaults when no env vars are set', () => {
    delete process.env.APERTURE_BASE_URL;
    delete process.env.APERTURE_POOL_SIZE;
    delete process.env.APERTURE_TIMEOUT;

    const cfg = loadConfig();

    expect(cfg.baseURL).toBe('http://localhost:8080');
    expect(cfg.poolSize).toBe(5);
    expect(cfg.timeoutMs).toBe(30000);
  });

  it('reads APERTURE_BASE_URL from environment', () => {
    process.env.APERTURE_BASE_URL = 'http://myhost:9090';
    const cfg = loadConfig();
    expect(cfg.baseURL).toBe('http://myhost:9090');
  });

  it('reads APERTURE_POOL_SIZE from environment', () => {
    process.env.APERTURE_POOL_SIZE = '10';
    const cfg = loadConfig();
    expect(cfg.poolSize).toBe(10);
  });

  it('reads APERTURE_TIMEOUT from environment', () => {
    process.env.APERTURE_TIMEOUT = '60000';
    const cfg = loadConfig();
    expect(cfg.timeoutMs).toBe(60000);
  });

  it('falls back to default for invalid APERTURE_POOL_SIZE', () => {
    process.env.APERTURE_POOL_SIZE = 'not-a-number';
    const cfg = loadConfig();
    expect(cfg.poolSize).toBe(5);
  });

  it('falls back to default for zero APERTURE_POOL_SIZE', () => {
    process.env.APERTURE_POOL_SIZE = '0';
    const cfg = loadConfig();
    expect(cfg.poolSize).toBe(5);
  });

  it('falls back to default for negative APERTURE_TIMEOUT', () => {
    process.env.APERTURE_TIMEOUT = '-1';
    const cfg = loadConfig();
    expect(cfg.timeoutMs).toBe(30000);
  });
});

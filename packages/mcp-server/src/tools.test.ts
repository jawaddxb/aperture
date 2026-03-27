/**
 * tools.test.ts — Unit tests for tool definitions.
 */

import { ALL_TOOLS, ToolName } from './tools';

describe('ALL_TOOLS', () => {
  const expectedNames: ToolName[] = ['execute', 'screenshot', 'navigation', 'pool_status'];

  it('contains exactly 4 tools', () => {
    expect(ALL_TOOLS).toHaveLength(4);
  });

  it('has all required tool names', () => {
    const names = ALL_TOOLS.map((t) => t.name);
    expect(names).toEqual(expect.arrayContaining(expectedNames));
  });

  it('every tool has a non-empty description', () => {
    for (const tool of ALL_TOOLS) {
      expect(typeof tool.description).toBe('string');
      expect(tool.description.length).toBeGreaterThan(10);
    }
  });

  it('every tool has an inputSchema with type=object', () => {
    for (const tool of ALL_TOOLS) {
      expect(tool.inputSchema.type).toBe('object');
    }
  });

  it('execute requires "goal"', () => {
    const exec = ALL_TOOLS.find((t) => t.name === 'execute')!;
    expect(exec.inputSchema.required).toContain('goal');
  });

  it('screenshot requires "url"', () => {
    const shot = ALL_TOOLS.find((t) => t.name === 'screenshot')!;
    expect(shot.inputSchema.required).toContain('url');
  });

  it('navigation requires "action" and "session_id"', () => {
    const nav = ALL_TOOLS.find((t) => t.name === 'navigation')!;
    expect(nav.inputSchema.required).toContain('action');
    expect(nav.inputSchema.required).toContain('session_id');
  });

  it('pool_status has no required fields', () => {
    const pool = ALL_TOOLS.find((t) => t.name === 'pool_status')!;
    expect(pool.inputSchema.required).toBeUndefined();
  });
});

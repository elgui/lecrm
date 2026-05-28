import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { api, ApiError } from './api';

describe('ApiError', () => {
  it('sets status and message', () => {
    const err = new ApiError(404, 'not found');
    expect(err.status).toBe(404);
    expect(err.message).toBe('not found');
    expect(err.name).toBe('ApiError');
  });

  it('extends Error', () => {
    const err = new ApiError(500, 'server error');
    expect(err).toBeInstanceOf(Error);
    expect(err.stack).toBeDefined();
  });
});

describe('api', () => {
  const mockFetch = vi.fn();

  beforeEach(() => {
    vi.stubGlobal('fetch', mockFetch);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('get sends GET with json content-type', async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ data: [] }),
    });

    const result = await api.get('/v1/contacts');
    expect(mockFetch).toHaveBeenCalledWith('/v1/contacts', {
      headers: { 'Content-Type': 'application/json' },
    });
    expect(result).toEqual({ data: [] });
  });

  it('post sends POST with JSON body', async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      status: 201,
      json: () => Promise.resolve({ id: '123' }),
    });

    const result = await api.post('/v1/contacts', { first_name: 'John' });
    expect(mockFetch).toHaveBeenCalledWith('/v1/contacts', {
      method: 'POST',
      body: JSON.stringify({ first_name: 'John' }),
      headers: { 'Content-Type': 'application/json' },
    });
    expect(result).toEqual({ id: '123' });
  });

  it('put sends PUT with JSON body', async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ id: '123', first_name: 'Jane' }),
    });

    await api.put('/v1/contacts/123', { first_name: 'Jane' });
    expect(mockFetch).toHaveBeenCalledWith('/v1/contacts/123', {
      method: 'PUT',
      body: JSON.stringify({ first_name: 'Jane' }),
      headers: { 'Content-Type': 'application/json' },
    });
  });

  it('delete sends DELETE', async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      status: 204,
    });

    const result = await api.delete('/v1/contacts/123');
    expect(mockFetch).toHaveBeenCalledWith('/v1/contacts/123', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
    });
    expect(result).toBeUndefined();
  });

  it('throws ApiError on non-ok response', async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      status: 422,
      text: () => Promise.resolve('{"error":"validation failed"}'),
    });

    await expect(api.get('/v1/contacts')).rejects.toThrow(ApiError);
    try {
      await api.get('/v1/contacts');
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      expect((e as ApiError).status).toBe(422);
    }
  });

  it('throws ApiError on 401', async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      status: 401,
      text: () => Promise.resolve('unauthorized'),
    });

    await expect(api.get('/auth/me')).rejects.toThrow(ApiError);
  });

  it('prepends /v1 to relative paths', async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({}),
    });

    await api.get('contacts');
    expect(mockFetch).toHaveBeenCalledWith('/v1/contacts', expect.any(Object));
  });

  it('does not prepend /v1 to absolute paths', async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({}),
    });

    await api.get('/auth/me');
    expect(mockFetch).toHaveBeenCalledWith('/auth/me', expect.any(Object));
  });
});

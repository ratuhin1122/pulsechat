export class ApiError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message);
    this.name = 'ApiError';
  }
}

export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return 'Could not connect. Check your connection and try again.';
}

export class HttpClient {
  constructor(private readonly fetcher: typeof fetch = (...args) => globalThis.fetch(...args)) {}

  async blob(path: string, token: string, signal?: AbortSignal): Promise<Blob> {
    const response = await this.fetcher(path, { signal, headers: { Authorization: `Bearer ${token}` },
      credentials: 'include', cache: 'no-store', redirect: 'error' });
    if (!response.ok) throw new ApiError(response.status, 'Could not load this file.');
    return response.blob();
  }

  async send<T>(path: string, init: RequestInit = {}): Promise<T> {
    const response = await this.fetcher(path, {
      ...init, credentials: 'include', cache: 'no-store', redirect: 'error',
    });
    if (!response.ok) {
      const text = (await response.text()).trim();
      throw new ApiError(response.status, response.status >= 500
        ? 'Something went wrong. Please try again.'
        : text || 'The request could not be completed.');
    }
    if (response.status === 204) return undefined as T;
    try { return await response.json() as T; }
    catch { throw new ApiError(502, 'The server returned an unexpected response.'); }
  }

  post<T>(path: string, body?: unknown): Promise<T> {
    return this.send<T>(path, {
      method: 'POST',
      ...(body === undefined ? {} : {
        headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
      }),
    });
  }
}

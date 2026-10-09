import { describe, expect, it } from 'vitest'
import {
  acceptLanguageFromEnv,
  Client,
  extractPlatformHost,
  isApiKey,
  isToken,
  problemMediaType,
  problemTypeUrl,
  toApiError,
  USER_TOKEN_PREFIX,
} from './index'

function respond(status: number, body: string, contentType: string) {
  const requests: Request[] = []
  const fetch = async (input: Request) => {
    requests.push(input)
    return new Response(body, {
      status,
      statusText: status === 502 ? 'Bad Gateway' : '',
      headers: { 'Content-Type': contentType },
    })
  }
  return { fetch, requests }
}

async function failedRequest(
  status: number,
  body: string,
  contentType = problemMediaType
) {
  const { fetch, requests } = respond(status, body, contentType)
  const client = new Client('https://activate.parallel.works', {
    fetch,
    acceptLanguage: 'ja-JP',
  }).withToken('a.b.c')
  const { error } = await client.GET('/api/workflows')
  return { error: toApiError(error), raw: error, request: requests[0]! }
}

describe('Client', () => {
  it('asks for problem details in the chosen language', async () => {
    const { request } = await failedRequest(404, '{}')
    expect(request.headers.get('Accept')).toContain(problemMediaType)
    expect(request.headers.get('Accept-Language')).toBe('ja-JP')
  })

  it('reads a problem', async () => {
    const { error, raw } = await failedRequest(
      404,
      JSON.stringify({
        type: '/problems/activate/workflow_not_found',
        status: 404,
        detail: 'ワークフローが見つかりません',
        code: 'workflow_not_found',
        params: { name: 'x' },
      })
    )
    expect(error.code).toBe('workflow_not_found')
    expect(error.status).toBe(404)
    expect(error.message).toBe('ワークフローが見つかりません')
    expect(error.params).toEqual({ name: 'x' })
    expect(raw).toMatchObject({ message: 'ワークフローが見つかりません' })
    expect(problemTypeUrl(error, 'https://activate.parallel.works')).toBe(
      'https://activate.parallel.works/problems/activate/workflow_not_found'
    )
  })

  it('derives the code of an about:blank problem from its status', async () => {
    const { error } = await failedRequest(
      403,
      JSON.stringify({ type: 'about:blank', status: 403, title: 'Forbidden' })
    )
    expect(error.code).toBe('forbidden')
    expect(problemTypeUrl(error, 'https://activate.parallel.works')).toBe(
      undefined
    )
  })

  it('reads the invalid fields of a validation problem', async () => {
    const { error } = await failedRequest(
      422,
      JSON.stringify({
        type: '/problems/activate/validation',
        status: 422,
        detail: 'validation failed',
        code: 'validation',
        errors: [
          {
            type: '/problems/activate/too_long',
            code: 'too_long',
            detail: 'too long',
            pointer: '#/items/0/name',
            params: { max: 3 },
          },
          {
            type: '/problems/activate/required',
            code: 'required',
            parameter: 'org',
            in: 'query',
          },
        ],
      })
    )
    expect(error.code).toBe('validation')
    expect(error.fields.map(f => [f.path, f.code, f.params])).toEqual([
      ['items[0].name', 'too_long', { max: 3 }],
      ['org', 'required', {}],
    ])
  })

  it('reads the older envelope', async () => {
    const { error } = await failedRequest(
      404,
      JSON.stringify({
        error: true,
        message: 'Workflow not found',
        code: 'workflow_not_found',
      }),
      'application/json'
    )
    expect(error.code).toBe('workflow_not_found')
    expect(error.message).toBe('Workflow not found')
  })

  it("reads a proxy's page", async () => {
    const { error } = await failedRequest(
      502,
      '<html><body>Bad Gateway</body></html>',
      'text/html'
    )
    expect(error.status).toBe(502)
    expect(error.code).toBe('internal')
    expect(error.message).toBe('Bad Gateway')
  })
})

describe('acceptLanguageFromEnv', () => {
  it.each([
    [{ LANG: 'ja_JP.UTF-8' }, 'ja-JP'],
    [{ LANG: 'de_DE.UTF-8@euro' }, 'de-DE'],
    [{ LANG: 'es' }, 'es'],
    [{ LANG: 'C' }, undefined],
    [{ LANG: 'C.UTF-8' }, undefined],
    [{ LANG: 'POSIX' }, undefined],
    [{}, undefined],
    [{ LC_MESSAGES: 'ko_KR.UTF-8', LANG: 'en_US.UTF-8' }, 'ko-KR'],
    [{ LC_ALL: 'fr_FR.UTF-8', LC_MESSAGES: 'ko_KR.UTF-8' }, 'fr-FR'],
  ])('%o is %s', (env, want) => {
    expect(acceptLanguageFromEnv(env)).toBe(want)
  })
})

describe('opaque user tokens', () => {
  const token = `${USER_TOKEN_PREFIX}${btoa('activate.parallel.works')}.${btoa('raw')}`

  it('is a token, not an API key', () => {
    expect(isToken(token)).toBe(true)
    expect(isApiKey(token)).toBe(false)
  })

  it('names its platform host', () => {
    expect(extractPlatformHost(token)).toBe('activate.parallel.works')
    expect(() => extractPlatformHost(`${USER_TOKEN_PREFIX}no-dot`)).toThrow()
  })

  it('is sent as a Bearer token to the host it names', async () => {
    const { fetch, requests } = respond(200, '[]', 'application/json')
    const client = Client.fromCredential(token, { fetch })
    await client.GET('/api/workflows')
    const request = requests[0]!
    expect(new URL(request.url).host).toBe('activate.parallel.works')
    expect(request.headers.get('Authorization')).toBe(`Bearer ${token}`)
  })
})

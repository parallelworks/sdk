import {
  chmodSync,
  existsSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { delimiter, join, relative } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  Client,
  CLIAuth,
  CredentialError,
  isToken,
  loadCredentialConfig,
  NoPlatformHostError,
  resolveCommand,
  resolveIdentity,
  SignInExpiredError,
} from './index'

let binDir: string
let credDir: string
let callLog: string

function credentialsPath() {
  return join(credDir, '.credentials')
}

function fakeCLI(body: string) {
  const script = `#!/bin/sh\necho "$* key=\${PW_API_KEY:-unset}" >> ${callLog}\n${body}\n`
  writeFileSync(join(binDir, 'pw'), script)
  chmodSync(join(binDir, 'pw'), 0o755)
}

function calls(): string[] {
  return existsSync(callLog)
    ? readFileSync(callLog, 'utf8').trim().split('\n')
    : []
}

function signIn(token: string, expiresInMs: number) {
  return JSON.stringify({
    currentIdentity: 'work',
    identities: {
      work: {
        token,
        server: 'work.example.com',
        name: 'work',
        organization: 'org',
        oauth: {
          clientId: 'pw-cli',
          expiresAt: new Date(Date.now() + expiresInMs).toISOString(),
          keychainAccount: 'work',
        },
      },
      other: {
        token: 'pwoa_other',
        server: 'other.example.com',
        name: 'other',
        organization: 'org',
        oauth: { expiresAt: new Date(Date.now() - 1000).toISOString() },
      },
      api: {
        apikey: 'pwt_aG9zdA==.key',
        server: 'api.example.com',
        name: 'api',
        organization: 'org',
      },
    },
  })
}

function writeSignIn(token: string, expiresInMs: number) {
  writeFileSync(credentialsPath(), signIn(token, expiresInMs))
}

// Prints a token response for token; no expiresIn leaves expires_in out.
function printScript(token: string, expiresIn?: number) {
  const response = { access_token: token, token_type: 'Bearer' }
  return `echo '${JSON.stringify(expiresIn === undefined ? response : { ...response, expires_in: expiresIn })}'`
}

function respond() {
  const requests: Request[] = []
  const fetch = async (input: Request) => {
    requests.push(input)
    return new Response('[]', {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  return { fetch, requests }
}

async function authorizations(
  client: ReturnType<typeof Client.fromCredentialConfig>,
  requests: Request[],
  times = 1
) {
  for (let i = 0; i < times; i++) {
    await client.GET('/api/workflows')
  }
  return requests.map(r => r.headers.get('Authorization'))
}

beforeEach(() => {
  binDir = mkdtempSync(join(tmpdir(), 'pw-bin-'))
  credDir = mkdtempSync(join(tmpdir(), 'pw-cred-'))
  callLog = join(binDir, 'calls')
  vi.stubEnv('PATH', `${binDir}${delimiter}${process.env['PATH']}`)
  vi.stubEnv('PW_CREDENTIALS_DIR', credDir)
  vi.stubEnv('PW_API_KEY', '')
  vi.stubEnv('PW_CONTEXT', '')
  vi.stubEnv('PW_PLATFORM_HOST', '')
})

afterEach(() => {
  vi.unstubAllEnvs()
  vi.useRealTimers()
  vi.restoreAllMocks()
  rmSync(binDir, { recursive: true, force: true })
  rmSync(credDir, { recursive: true, force: true })
})

describe.skipIf(process.platform === 'win32')(
  'Client.fromCredentialConfig',
  () => {
    it('caches the token until it nears expiry', async () => {
      writeSignIn('pwoa_stored', 3_600_000)
      fakeCLI(printScript('pwoa_new', 3600))
      const { fetch, requests } = respond()
      const client = Client.fromCredentialConfig({ fetch })
      vi.stubEnv('PW_API_KEY', 'pwt_leaked')

      expect(await authorizations(client, requests, 3)).toEqual(
        Array(3).fill('Bearer pwoa_new')
      )
      expect(new URL(requests[0]!.url).host).toBe('work.example.com')
      expect(calls()).toEqual([
        'auth token --print -o json --context work key=unset',
      ])
    })

    it('renews the token before it expires', async () => {
      writeSignIn('pwoa_stored', 3_600_000)
      fakeCLI(printScript('pwoa_new', 3600))
      const { fetch, requests } = respond()
      const client = Client.fromCredentialConfig({ fetch })
      await client.GET('/api/workflows')

      vi.useFakeTimers({ toFake: ['Date'] })
      vi.setSystemTime(Date.now() + 3_570_000)
      fakeCLI(printScript('pwoa_newer', 3600))
      expect(await authorizations(client, requests, 2)).toEqual([
        'Bearer pwoa_new',
        'Bearer pwoa_newer',
        'Bearer pwoa_newer',
      ])
      expect(calls()).toHaveLength(2)
    })

    it('uses a token with no expiry for the life of the client', async () => {
      writeSignIn('pwoa_stored', 3_600_000)
      fakeCLI(printScript('pwoa_new'))
      const { fetch, requests } = respond()
      const client = Client.fromCredentialConfig({ fetch })

      expect(await authorizations(client, requests, 3)).toEqual(
        Array(3).fill('Bearer pwoa_new')
      )
      expect(calls()).toHaveLength(1)
    })

    it('asks the CLI for the context the SDK selected', async () => {
      writeSignIn('pwoa_old', 3_600_000)
      fakeCLI(printScript('pwoa_printed', 3600))
      vi.stubEnv('PW_CONTEXT', 'work')
      const { fetch, requests } = respond()
      const client = Client.fromCredentialConfig({ fetch, context: 'other' })

      expect(await authorizations(client, requests)).toEqual([
        'Bearer pwoa_printed',
      ])
      expect(calls()).toEqual([
        'auth token --print -o json --context other key=unset',
      ])
    })

    it('keeps a token the CLI could not renew until it expires', async () => {
      writeSignIn('pwoa_stored', 30_000)
      fakeCLI(printScript('pwoa_stored', 30))
      const { fetch, requests } = respond()
      const client = Client.fromCredentialConfig({ fetch })

      expect(await authorizations(client, requests, 3)).toEqual(
        Array(3).fill('Bearer pwoa_stored')
      )
      expect(calls()).toHaveLength(1)
    })

    it('uses the last token until it expires when the CLI fails', async () => {
      fakeCLI(printScript('pwoa_new', 3600))
      const auth = new CLIAuth({ context: 'work' })
      expect(await auth.token()).toBe('pwoa_new')

      fakeCLI('exit 1')
      vi.useFakeTimers({ toFake: ['Date'] })
      vi.setSystemTime(Date.now() + 3_570_000)
      expect(await auth.token()).toBe('pwoa_new')
      vi.setSystemTime(Date.now() + 60_000)
      await expect(auth.token()).rejects.toThrow(SignInExpiredError)
    })

    it('fails without the CLI', async () => {
      writeSignIn('pwoa_stored', 3_600_000)
      const { fetch } = respond()
      const client = Client.fromCredentialConfig({
        fetch,
        cliCommand: join(binDir, 'missing-pw'),
      })
      await expect(client.GET('/api/workflows')).rejects.toThrow(
        SignInExpiredError
      )
    })

    it("surfaces the CLI's error", async () => {
      fakeCLI("echo 'sign-in revoked' >&2\nexit 1")
      const error = await new CLIAuth({ context: 'work' })
        .token()
        .catch((e: unknown) => e)
      expect(error).toBeInstanceOf(SignInExpiredError)
      expect((error as Error).message).toContain('sign-in revoked')
    })

    it.each([
      ['a bare token', 'echo pwoa_bare'],
      ['no token', `echo '{"token_type":"Bearer"}'`],
      [
        'another token type',
        `echo '{"access_token":"pwoa_x","token_type":"mac"}'`,
      ],
      [
        'a spaced token',
        `echo '{"access_token":"pwoa x","token_type":"Bearer"}'`,
      ],
    ])('rejects output with %s', async (_, body) => {
      fakeCLI(body)
      await expect(new CLIAuth().token()).rejects.toThrow(SignInExpiredError)
    })

    it('bounds a run of the CLI', async () => {
      fakeCLI('exec sleep 5')
      const auth = new CLIAuth({ context: 'work', timeoutMs: 100 })
      await expect(auth.token()).rejects.toThrow('did not finish within 100ms')
    })

    it('refuses a pw found through a relative PATH entry', async () => {
      fakeCLI(printScript('pwoa_new', 3600))
      vi.stubEnv('PATH', relative(process.cwd(), binDir))
      expect(() => resolveCommand('pw')).toThrow(
        /relative to the current directory/
      )
      await expect(new CLIAuth().token()).rejects.toThrow(SignInExpiredError)
      expect(calls()).toEqual([])

      vi.stubEnv('PATH', binDir)
      expect(resolveCommand('pw')).toBe(join(binDir, 'pw'))
      expect(resolveCommand('./pw')).toBe('./pw')
      expect(() => resolveCommand('missing-pw')).toThrow(/not found on PATH/)
    })

    it('uses an API key context or PW_API_KEY as is', async () => {
      fakeCLI('echo pwoa_unexpected')
      writeSignIn('pwoa_stored', 3_600_000)
      const { fetch, requests } = respond()

      await Client.fromCredentialConfig({ fetch, context: 'api' }).GET(
        '/api/workflows'
      )
      vi.stubEnv('PW_API_KEY', 'pwoa_explicit')
      await Client.fromCredentialConfig({ fetch }).GET('/api/workflows')

      expect(requests.map(r => r.headers.get('Authorization'))).toEqual([
        `Basic ${btoa('pwt_aG9zdA==.key:')}`,
        'Bearer pwoa_explicit',
      ])
      expect(new URL(requests[1]!.url).host).toBe('work.example.com')
      expect(calls()).toEqual([])
    })
  }
)

describe('credentials file', () => {
  it('resolves PW_API_KEY, then the context option, PW_CONTEXT and the current context', () => {
    writeSignIn('pwoa_stored', 3_600_000)
    const config = loadCredentialConfig()

    expect(resolveIdentity(config).context).toBe('work')
    vi.stubEnv('PW_CONTEXT', 'other')
    expect(resolveIdentity(config).context).toBe('other')
    expect(resolveIdentity(config, { context: 'api' }).context).toBe('api')
    expect(() => resolveIdentity(config, { context: 'nope' })).toThrow(
      CredentialError
    )
    vi.stubEnv('PW_API_KEY', 'pwt_aG9zdA==.key')
    expect(resolveIdentity(config)).toEqual({
      context: '',
      identity: {
        apikey: 'pwt_aG9zdA==.key',
        server: 'host',
        name: '',
        organization: '',
      },
    })
  })

  it('reads a missing or unparseable file as no contexts', () => {
    expect(loadCredentialConfig().identities).toEqual({})
    writeFileSync(credentialsPath(), '{not json')
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    expect(loadCredentialConfig().identities).toEqual({})
  })

  it('reads the XDG path when PW_CREDENTIALS_DIR is unset', () => {
    const xdg = join(credDir, 'xdg')
    vi.stubEnv('PW_CREDENTIALS_DIR', '')
    vi.stubEnv('XDG_CONFIG_HOME', xdg)
    expect(loadCredentialConfig().path).toBe(join(xdg, 'pw', 'credentials'))
  })
})

describe('access tokens', () => {
  it('is a token that names no host', () => {
    expect(isToken('pwoa_abc')).toBe(true)
    expect(() => Client.fromCredential('pwoa_abc')).toThrow(NoPlatformHostError)
  })

  it('goes to PW_PLATFORM_HOST, else the selected context', async () => {
    writeSignIn('pwoa_stored', 3_600_000)
    const { fetch, requests } = respond()

    await Client.fromCredential('pwoa_abc', { fetch }).GET('/api/workflows')
    vi.stubEnv('PW_PLATFORM_HOST', 'env.example.com')
    await Client.fromCredential('pwoa_abc', { fetch }).GET('/api/workflows')

    expect(requests.map(r => new URL(r.url).host)).toEqual([
      'work.example.com',
      'env.example.com',
    ])
    expect(requests[0]!.headers.get('Authorization')).toBe('Bearer pwoa_abc')
  })
})

import {
  CredentialError,
  extractPlatformHost,
  isToken,
  NoPlatformHostError,
  SignInExpiredError,
} from './index'

/** A saved context in the pw credentials file, as the CLI writes it. */
export interface Identity {
  apikey?: string
  token?: string
  server: string
  name: string
  canonicalName?: string
  organization: string
  /** The state of a `pw auth` sign-in, which only the CLI reads. */
  oauth?: Record<string, unknown>
}

/** The pw credentials file. */
export interface CredentialConfig {
  identities: Record<string, Identity>
  currentIdentity: string
}

export interface ResolveOptions {
  /** The context to use, over PW_CONTEXT and the current context. */
  context?: string | undefined
  /** The platform host to use, over the context's server. */
  platformHost?: string | undefined
}

type Env = Record<string, string | undefined>

interface NodeFs {
  readFileSync(path: string, encoding: 'utf8'): string
  existsSync(path: string): boolean
  statSync(
    path: string,
    options: { throwIfNoEntry: false }
  ): { isFile(): boolean } | undefined
  accessSync(path: string, mode: number): void
  constants: { X_OK: number }
}

interface NodePath {
  join(...parts: string[]): string
  isAbsolute(path: string): boolean
  delimiter: string
}

interface NodeOs {
  homedir(): string
}

interface ExecFileError extends Error {
  code?: string | number
  killed?: boolean
  signal?: string | null
}

interface NodeChildProcess {
  execFile(
    file: string,
    args: string[],
    options: { env: Env; timeout: number; encoding: 'utf8' },
    callback: (
      error: ExecFileError | null,
      stdout: string,
      stderr: string
    ) => void
  ): unknown
}

interface NodeProcess {
  env: Env
  platform?: string
  getBuiltinModule?: (id: string) => unknown
}

function nodeProcess(): NodeProcess | undefined {
  return (globalThis as { process?: NodeProcess }).process
}

function env(): Env {
  return nodeProcess()?.env ?? {}
}

// getBuiltinModule rather than an import keeps this module loadable in a browser bundle.
function builtin<T>(id: string): T {
  const get = nodeProcess()?.getBuiltinModule
  if (!get) {
    throw new CredentialError(
      `reading pw credentials needs Node.js 20.16 or later (${id})`
    )
  }
  return get(id) as T
}

/** The pw executable CLIAuth runs, looked up on PATH. */
export const DEFAULT_CLI_COMMAND = 'pw'

/** One minute, as the AWS SDK's credential_process provider bounds a run by default. */
export const DEFAULT_CLI_TIMEOUT_MS = 60_000

// Inside the CLI's two-minute renewal window, so the CLI always renews a token the SDK asks it for.
const CLI_RENEW_MARGIN_MS = 60_000

/**
 * The credentials file path, ~/.config/pw/credentials (XDG_CONFIG_HOME moves
 * it, PW_CREDENTIALS_DIR replaces its directory), as the CLI and Go SDK resolve it.
 */
export function defaultCredentialConfigPath(): string {
  const path = builtin<NodePath>('node:path')
  const override = env()['PW_CREDENTIALS_DIR']
  if (override) {
    return path.join(override, '.credentials')
  }
  const xdg = env()['XDG_CONFIG_HOME']
  const base =
    xdg && path.isAbsolute(xdg)
      ? xdg
      : path.join(builtin<NodeOs>('node:os').homedir(), '.config')
  return path.join(base, 'pw', 'credentials')
}

function legacyCredentialConfigPath(): string {
  return builtin<NodePath>('node:path').join(
    builtin<NodeOs>('node:os').homedir(),
    'pw',
    '.credentials'
  )
}

/**
 * Reads the credentials file, falling back to the legacy ~/pw/.credentials.
 * A missing, empty or unparseable file reads as no contexts.
 */
export function loadCredentialConfig(
  path?: string
): CredentialConfig & { path: string } {
  const fs = builtin<NodeFs>('node:fs')
  if (!path) {
    path = defaultCredentialConfigPath()
    if (!fs.existsSync(path) && !env()['PW_CREDENTIALS_DIR']) {
      const legacy = legacyCredentialConfigPath()
      if (fs.existsSync(legacy)) {
        path = legacy
      }
    }
  }
  const empty = { identities: {}, currentIdentity: '', path }
  let data: string
  try {
    data = fs.readFileSync(path, 'utf8')
  } catch (e) {
    if ((e as ExecFileError).code === 'ENOENT') {
      return empty
    }
    throw e
  }
  if (!data.trim()) {
    return empty
  }
  try {
    const parsed = JSON.parse(data) as Partial<CredentialConfig>
    return {
      identities: parsed.identities ?? {},
      currentIdentity: parsed.currentIdentity ?? '',
      path,
    }
  } catch {
    console.warn(
      `warning: credentials file (${path}) is unparseable, treating as missing; re-authenticate with 'pw auth' to recover`
    )
    return empty
  }
}

/** The context the CLI uses too: the option, then PW_CONTEXT, then the current context. */
function selectedContext(config: CredentialConfig, override?: string): string {
  return override || env()['PW_CONTEXT'] || config.currentIdentity
}

/**
 * The identity a client authenticates as, and the context it came from (empty
 * for PW_API_KEY): PW_API_KEY, then the context option, PW_CONTEXT, and the
 * file's current context, as the CLI and Go SDK pick it.
 */
export function resolveIdentity(
  config: CredentialConfig,
  options: ResolveOptions = {}
): { context: string; identity: Identity } {
  const credential = env()['PW_API_KEY']
  if (credential) {
    return {
      context: '',
      identity: identityFromCredential(config, credential, options),
    }
  }
  const context = selectedContext(config, options.context)
  if (!context) {
    throw new CredentialError(
      "no context configured; use 'pw auth' to authenticate"
    )
  }
  const identity = config.identities[context]
  if (!identity) {
    throw new CredentialError(`context "${context}" not found`)
  }
  return {
    context,
    identity: options.platformHost
      ? { ...identity, server: options.platformHost }
      : { ...identity },
  }
}

// A token that names no platform goes to PW_PLATFORM_HOST, else the selected context's server.
function identityFromCredential(
  config: CredentialConfig,
  credential: string,
  options: ResolveOptions
): Identity {
  let host = options.platformHost
  if (!host) {
    try {
      host = extractPlatformHost(credential)
    } catch (e) {
      if (!(e instanceof NoPlatformHostError)) {
        throw e
      }
      host =
        env()['PW_PLATFORM_HOST'] ||
        config.identities[selectedContext(config, options.context)]?.server
      if (!host) {
        throw e
      }
    }
  }
  const identity: Identity = { server: host, name: '', organization: '' }
  if (isToken(credential)) {
    identity.token = credential
  } else {
    identity.apikey = credential
  }
  return identity
}

/** The platform host for a credential that names none, or undefined outside Node.js. */
export function hostForUnnamedCredential(): string | undefined {
  const fromEnv = env()['PW_PLATFORM_HOST']
  if (fromEnv) {
    return fromEnv
  }
  if (!nodeProcess()?.getBuiltinModule) {
    return undefined
  }
  const config = loadCredentialConfig()
  return config.identities[selectedContext(config)]?.server || undefined
}

/** Supplies the Bearer token for each request. */
export interface TokenProvider {
  token(): Promise<string>
}

export interface CLIAuthOptions {
  /** The pw executable, looked up on PATH unless it is a path. */
  command?: string | undefined
  /** The context to print a token for; empty means the one the CLI picks. */
  context?: string | undefined
  /** Bounds one run of the CLI. */
  timeoutMs?: number | undefined
}

/**
 * Authenticates as a `pw auth` sign-in. It runs `pw auth token --print -o json`
 * for a current access token instead of rotating the refresh token itself: the
 * platform signs a device out when two clients rotate the same refresh token,
 * and the CLI serializes its rotations across processes. Like a client-go exec
 * credential plugin or an AWS credential_process, it reads the token and its
 * lifetime from the command's stdout, an RFC 6749 section 5.1 token response,
 * and caches the token in memory until it nears expiry.
 */
export class CLIAuth implements TokenProvider {
  private readonly options: CLIAuthOptions
  private cached = ''
  // Undefined when the CLI did not say, so the token is used for the life of the client.
  private expiresAt: number | undefined
  // A token the CLI could not renew is used until it expires rather than asking on every request.
  private settled = false
  private pending: Promise<string> | undefined

  constructor(options: CLIAuthOptions = {}) {
    this.options = options
  }

  token(): Promise<string> {
    if (this.usable()) {
      return Promise.resolve(this.cached)
    }
    this.pending ??= this.renew().finally(() => {
      this.pending = undefined
    })
    return this.pending
  }

  private usable(): boolean {
    if (!this.cached) {
      return false
    }
    if (this.expiresAt === undefined) {
      return true
    }
    if (this.settled) {
      return Date.now() < this.expiresAt
    }
    return this.expiresAt - Date.now() > CLI_RENEW_MARGIN_MS
  }

  private async renew(): Promise<string> {
    let printed: { token: string; expiresAt: number | undefined }
    try {
      printed = await this.run()
    } catch (e) {
      if (
        this.cached &&
        this.expiresAt !== undefined &&
        Date.now() < this.expiresAt
      ) {
        this.settled = true
        return this.cached
      }
      throw new SignInExpiredError(e)
    }
    this.cached = printed.token
    this.expiresAt = printed.expiresAt
    this.settled =
      this.expiresAt !== undefined &&
      this.expiresAt - Date.now() <= CLI_RENEW_MARGIN_MS
    return this.cached
  }

  private run(): Promise<{ token: string; expiresAt: number | undefined }> {
    const command = this.options.command || DEFAULT_CLI_COMMAND
    const timeout = this.options.timeoutMs || DEFAULT_CLI_TIMEOUT_MS
    const args = ['auth', 'token', '--print', '-o', 'json']
    if (this.options.context) {
      args.push('--context', this.options.context)
    }
    const describe = [command, ...args].join(' ')
    const file = resolveCommand(command)
    // Without PW_API_KEY, which would make the CLI print it rather than the sign-in's token.
    const { PW_API_KEY: _, ...childEnv } = env()
    const { execFile } = builtin<NodeChildProcess>('node:child_process')
    return new Promise((resolve, reject) => {
      execFile(
        file,
        args,
        { env: childEnv, timeout, encoding: 'utf8' },
        (error, stdout, stderr) => {
          if (error) {
            reject(cliFailure(describe, timeout, error, stderr))
            return
          }
          try {
            resolve(parseTokenResponse(describe, stdout, Date.now()))
          } catch (e) {
            reject(e)
          }
        }
      )
    })
  }
}

function parseTokenResponse(
  describe: string,
  stdout: string,
  received: number
): { token: string; expiresAt: number | undefined } {
  let response: {
    access_token?: unknown
    token_type?: unknown
    expires_in?: unknown
  }
  try {
    response = JSON.parse(stdout)
  } catch (e) {
    throw new Error(`${describe} printed no token response: ${e}`)
  }
  const token = response.access_token
  if (typeof token !== 'string' || !token || /\s/.test(token)) {
    throw new Error(`${describe} printed no single access_token`)
  }
  if (
    typeof response.token_type !== 'string' ||
    response.token_type.toLowerCase() !== 'bearer'
  ) {
    throw new Error(
      `${describe} printed token_type ${JSON.stringify(response.token_type)}, want Bearer`
    )
  }
  const expiresIn = response.expires_in
  return {
    token,
    expiresAt:
      typeof expiresIn === 'number' ? received + expiresIn * 1000 : undefined,
  }
}

/**
 * Finds command on PATH as Go's exec.LookPath does: a command that names a
 * path is used as is, and a match through a relative PATH entry, such as the
 * current directory, is refused. Windows' own lookup would search the current
 * directory first.
 */
export function resolveCommand(command: string): string {
  const windows = nodeProcess()?.platform === 'win32'
  if (command.includes('/') || (windows && /[\\:]/.test(command))) {
    return command
  }
  const fs = builtin<NodeFs>('node:fs')
  const path = builtin<NodePath>('node:path')
  const extensions = windows ? windowsExtensions(command) : ['']
  for (const dir of (env()['PATH'] ?? '').split(path.delimiter)) {
    for (const ext of extensions) {
      const candidate = path.join(dir || '.', command + ext)
      if (!isExecutable(fs, candidate, windows)) {
        continue
      }
      if (!path.isAbsolute(candidate)) {
        throw new CredentialError(
          `${command} resolves to ${candidate}, relative to the current directory; give the pw command as an absolute path to run it`
        )
      }
      return candidate
    }
  }
  throw new CredentialError(`${command}: not found on PATH`)
}

// The PATHEXT extensions to try, after the command as is when it already has one.
function windowsExtensions(command: string): string[] {
  const extensions = (env()['PATHEXT'] || '.COM;.EXE;.BAT;.CMD')
    .split(';')
    .filter(Boolean)
  const named = extensions.some(ext =>
    command.toLowerCase().endsWith(ext.toLowerCase())
  )
  return named ? ['', ...extensions] : extensions
}

function isExecutable(fs: NodeFs, file: string, windows: boolean): boolean {
  if (!fs.statSync(file, { throwIfNoEntry: false })?.isFile()) {
    return false
  }
  if (windows) {
    return true
  }
  try {
    fs.accessSync(file, fs.constants.X_OK)
    return true
  } catch {
    return false
  }
}

function cliFailure(
  describe: string,
  timeout: number,
  error: ExecFileError,
  stderr: string
): Error {
  if (error.killed) {
    return new Error(`${describe} did not finish within ${timeout}ms`)
  }
  const reason =
    error.code === 'ENOENT' ? 'not found' : `exit status ${error.code}`
  const detail = stderr.trim()
  return new Error(`${describe}: ${reason}${detail ? `: ${detail}` : ''}`)
}

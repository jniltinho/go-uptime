// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  MAX_RESTORE_FILE_BYTES,
  buildRestoreBody,
  describeBackupError,
  detectBackupFormat,
  errorFromResponseText,
  filenameFromContentDisposition,
  filterPlanItems,
  formatWarning,
  isRestoreFileTooLarge,
  parseRetryAfter,
  passwordByteLength,
  planCounts,
  readBlobResponse,
  restoreConfirmationMessage,
  resultCounts,
  validatePassword,
} from './adminBackup.js'

test('detects the format of the parsed file', () => {
  assert.equal(detectBackupFormat({ format: 'go-uptime-admin-backup', version: 1 }), 'plain')
  assert.equal(detectBackupFormat({ format: 'go-uptime-admin-backup-encrypted', version: 1 }), 'encrypted')
  // The files written up to v6, while the project was called Gatus, are still restored
  assert.equal(detectBackupFormat({ format: 'gatus-admin-backup', version: 1 }), 'plain')
  assert.equal(detectBackupFormat({ format: 'gatus-admin-backup-encrypted', version: 1 }), 'encrypted')
  for (const value of [null, undefined, 'go-uptime-admin-backup', 42, [], [{ format: 'go-uptime-admin-backup' }], {}, { format: 'other' }, { format: 'GO-UPTIME-ADMIN-BACKUP' }]) {
    assert.equal(detectBackupFormat(value), 'unknown')
  }
})

test('limits the size of the restored file', () => {
  assert.equal(MAX_RESTORE_FILE_BYTES, 2.7 * 1024 * 1024)
  assert.equal(isRestoreFileTooLarge(MAX_RESTORE_FILE_BYTES), false)
  assert.equal(isRestoreFileTooLarge(Math.ceil(2.67 * 1024 * 1024)), false)
  assert.equal(isRestoreFileTooLarge(Math.floor(MAX_RESTORE_FILE_BYTES) + 1), true)
})

test('counts the password in UTF-8 bytes and validates it', () => {
  assert.equal(passwordByteLength('abc'), 3)
  assert.equal(passwordByteLength('ção'), 5)
  assert.equal(passwordByteLength('🔒'), 4)
  assert.equal(passwordByteLength(undefined), 0)
  assert.match(validatePassword('short'), /at least 12 bytes/)
  // 6 characters, 12 bytes
  assert.equal(validatePassword('çççççç'), '')
  assert.equal(validatePassword('a'.repeat(1024)), '')
  assert.match(validatePassword('a'.repeat(1025)), /at most 1024 bytes/)
  assert.match(validatePassword('ç'.repeat(513)), /at most 1024 bytes/)
  assert.match(validatePassword('correct horse battery', 'correct horse batterY'), /do not match/)
  assert.equal(validatePassword('correct horse battery', 'correct horse battery'), '')
})

test('reads the name of the file from Content-Disposition', () => {
  assert.equal(filenameFromContentDisposition('attachment; filename="go-uptime-backup-20260916-180000.enc.json"'), 'go-uptime-backup-20260916-180000.enc.json')
  assert.equal(filenameFromContentDisposition('attachment; filename=go-uptime-backup-20260916-180000.json'), 'go-uptime-backup-20260916-180000.json')
  assert.equal(filenameFromContentDisposition("attachment; filename*=UTF-8''go-uptime%20backup.json; filename=\"x.json\""), 'go-uptime backup.json')
  assert.equal(filenameFromContentDisposition('attachment; filename="../../etc/passwd"'), 'passwd')
  assert.equal(filenameFromContentDisposition('attachment; filename=""'), 'go-uptime-backup.json')
  assert.equal(filenameFromContentDisposition('attachment'), 'go-uptime-backup.json')
  assert.equal(filenameFromContentDisposition(null), 'go-uptime-backup.json')
  assert.equal(filenameFromContentDisposition(undefined, 'other.json'), 'other.json')
})

const plan = {
  summary: { create: 3, update: 1, unchanged: 5, skip: 2 },
  notices: { monitoringStarts: 3, withAlerts: 2 },
  fingerprint: 'abc',
  items: [
    { type: 'pushKey', id: 'akamai', action: 'create', reason: '', warnings: [] },
    { type: 'endpoint', id: 'jobs_backup', action: 'update', reason: 'name or group changes', warnings: [] },
    { type: 'endpoint', id: 'core_api', action: 'skip', reason: 'already exists', warnings: [] },
    { type: 'statusPage', id: 'jobs', action: 'create', reason: '', warnings: ['group jobs has no endpoints'] },
  ],
}

test('counts and filters the plan', () => {
  assert.deepEqual(planCounts(plan), { create: 3, update: 1, unchanged: 5, skip: 2 })
  assert.deepEqual(planCounts({ items: plan.items }), { create: 2, update: 1, unchanged: 0, skip: 1 })
  assert.deepEqual(planCounts(null), { create: 0, update: 0, unchanged: 0, skip: 0 })
  assert.equal(filterPlanItems(plan.items, 'all').length, 4)
  assert.equal(filterPlanItems(plan.items, '').length, 4)
  assert.deepEqual(filterPlanItems(plan.items, 'create').map((item) => item.id), ['akamai', 'jobs'])
  assert.deepEqual(filterPlanItems(plan.items, 'unchanged'), [])
  assert.deepEqual(filterPlanItems(undefined, 'skip'), [])
})

test('confirms with the quantities of the plan', () => {
  assert.equal(restoreConfirmationMessage(plan), '3 items will be created and 1 updated.')
  assert.equal(restoreConfirmationMessage({ summary: { create: 1, update: 0 } }), '1 item will be created and 0 updated.')
})

test('counts the results of the restore', () => {
  const results = [
    { type: 'pushKey', id: 'akamai', result: 'created' },
    { type: 'endpoint', id: 'a', result: 'failed', message: 'changed during the restore' },
    { type: 'endpoint', id: 'b', result: 'skipped' },
    { type: 'statusPage', id: 'jobs', result: 'created' },
    { type: 'statusPage', id: 'x', result: 'other' },
  ]
  assert.deepEqual(resultCounts(results), { created: 2, updated: 0, unchanged: 0, skipped: 1, failed: 1 })
  assert.deepEqual(resultCounts(null), { created: 0, updated: 0, unchanged: 0, skipped: 0, failed: 0 })
})

test('formats the warnings', () => {
  assert.equal(formatWarning('plain'), 'plain')
  assert.equal(formatWarning({ message: 'with message' }), 'with message')
  assert.equal(formatWarning({ type: 'group', value: 'jobs' }), 'group: jobs')
})

test('builds the body of the preview and of the restore', () => {
  const file = { format: 'go-uptime-admin-backup', version: 1 }
  assert.deepEqual(buildRestoreBody({ file, format: 'plain', password: 'ignored', overwrite: true }), { file, overwrite: true, disableEndpoints: false })
  assert.deepEqual(
    buildRestoreBody({ file, format: 'encrypted', password: 'correct horse battery', disableEndpoints: true, fingerprint: 'f' }),
    { file, password: 'correct horse battery', overwrite: false, disableEndpoints: true, fingerprint: 'f' }
  )
})

test('parses Retry-After in seconds', () => {
  assert.equal(parseRetryAfter('5'), 5)
  assert.equal(parseRetryAfter(' 30 '), 30)
  assert.equal(parseRetryAfter(7), 7)
  assert.equal(parseRetryAfter('Wed, 21 Oct 2026 07:28:00 GMT'), null)
  assert.equal(parseRetryAfter(null), null)
})

test('reads the JSON or text of an error answer', () => {
  assert.deepEqual(errorFromResponseText(400, 'Bad Request', '{"error":"password required"}', null), {
    status: 400, message: 'password required', data: { error: 'password required' }, retryAfter: null,
  })
  assert.deepEqual(errorFromResponseText(413, 'Request Entity Too Large', 'Request Entity Too Large', null), {
    status: 413, message: 'Request Entity Too Large', data: null, retryAfter: null,
  })
  assert.equal(errorFromResponseText(502, 'Bad Gateway', '', null).message, 'Bad Gateway')
  assert.equal(errorFromResponseText(429, '', '{"error":"too many"}', '5').retryAfter, 5)
})

test('describes the errors of the backup and of the restore', () => {
  assert.match(describeBackupError({ status: 413, message: 'Request Entity Too Large' }), /too large \(maximum 2 MiB/)
  assert.match(describeBackupError({ status: 413 }), /proxy/)
  assert.match(describeBackupError({ status: 422, data: { error: 'backup exceeds 2 MiB or the item limits' } }), /item limits/)
  assert.match(describeBackupError({ status: 429, retryAfter: 5 }), /Try again in 5 seconds\./)
  assert.match(describeBackupError({ status: 429, retryAfter: '1' }), /Try again in 1 second\./)
  assert.match(describeBackupError({ status: 429 }), /Try again later\./)
  assert.equal(describeBackupError({ status: 400, data: { error: 'invalid password or corrupted file' } }), 'Invalid password or corrupted file.')
  assert.equal(describeBackupError({ status: 400, message: 'Bad Request' }), 'The request or the backup file is invalid.')
  assert.match(describeBackupError({ status: 409, data: { error: 'plan changed' } }), /^Plan changed\. .*run Preview again/)
  assert.match(describeBackupError({ status: 409 }), /run Preview again/)
  assert.equal(describeBackupError({ status: 415, data: { error: 'content type must be application/json' } }), 'Content type must be application/json.')
  assert.match(describeBackupError({ status: 503 }), /reloading its configuration/)
  assert.match(describeBackupError({ status: 503, data: { error: 'managed endpoints unavailable' } }), /^Managed endpoints unavailable\. Try again/)
  assert.equal(describeBackupError({ status: 401 }), 'Authentication required.')
  assert.equal(describeBackupError({ status: 500, message: 'boom' }), 'boom')
  assert.equal(describeBackupError(null), 'Unexpected error.')
})

test('reads the Blob and the name of a download', async () => {
  const response = new Response('{"format":"go-uptime-admin-backup"}', {
    status: 200,
    headers: { 'Content-Disposition': 'attachment; filename="go-uptime-backup-20260916-180000.json"' },
  })
  const { blob, filename } = await readBlobResponse(response)
  assert.equal(filename, 'go-uptime-backup-20260916-180000.json')
  assert.equal(await blob.text(), '{"format":"go-uptime-admin-backup"}')
  const fallback = await readBlobResponse(new Response('x', { status: 200 }))
  assert.equal(fallback.filename, 'go-uptime-backup.json')
})

test('throws the errors of a download and notifies a 401', async () => {
  let unauthorized = 0
  const createError = (status, message, options) => Object.assign(new Error(message), { status, ...options })
  await assert.rejects(
    readBlobResponse(new Response('{"error":"unauthorized"}', { status: 401 }), { onUnauthorized: () => unauthorized++, createError }),
    (error) => error.status === 401 && error.message === 'unauthorized'
  )
  assert.equal(unauthorized, 1)
  await assert.rejects(
    readBlobResponse(new Response('Request Entity Too Large', { status: 413, statusText: 'Request Entity Too Large' }), { createError }),
    (error) => error.status === 413 && error.message === 'Request Entity Too Large' && error.data === null
  )
  await assert.rejects(
    readBlobResponse(new Response('{"error":"busy"}', { status: 429, headers: { 'Retry-After': '5' } })),
    (error) => error.status === 429 && error.retryAfter === 5 && error.data.error === 'busy'
  )
})

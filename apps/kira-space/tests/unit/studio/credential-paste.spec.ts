// Credential-paste parser: several interacting label/separator/quote/delimiter rules, plus CLI
// flag tables. One table of cases pins the interactions; a few focused cases cover result shape.
import { describe, expect, test } from 'bun:test';
import type { ConnectionKind } from '../../../../../packages/shared/domain/connection';
import {
  MAX_PASTE_BYTES,
  type PasteField,
  parseCredentialPaste,
} from '../../../frontend/src/studio/project/credentialPaste/parse';

type Expected = Partial<Record<PasteField, string[]>>;

function values(input: string, kind: ConnectionKind): Expected {
  const out: Expected = {};
  for (const [k, list] of Object.entries(parseCredentialPaste(input, kind).fields)) {
    out[k as PasteField] = list.map((c) => c.value);
  }
  return out;
}

interface Row {
  name: string;
  kind?: ConnectionKind;
  input: string;
  expect: Expected;
}

const ROWS: Row[] = [
  {
    name: 'two labels on a line',
    input: 'user: foo  pass: bar',
    expect: { username: ['foo'], password: ['bar'] },
  },
  {
    name: 'single-space pair with explicit separators',
    input: 'user: foo pass: bar',
    expect: { username: ['foo'], password: ['bar'] },
  },
  {
    name: 'equals lines',
    input: 'username=foo\npassword=bar',
    expect: { username: ['foo'], password: ['bar'] },
  },
  {
    name: 'arrow separator',
    input: 'login -> foo\npwd => bar',
    expect: { username: ['foo'], password: ['bar'] },
  },
  {
    name: 'tab separator',
    input: 'user\tfoo\npassword\tbar',
    expect: { username: ['foo'], password: ['bar'] },
  },
  {
    name: 'whitespace separator at line start',
    input: 'user foo\npassword bar',
    expect: { username: ['foo'], password: ['bar'] },
  },
  { name: 'prose mid-line is not a label', input: 'my password is hunter2', expect: {} },
  {
    name: 'quoted value keeps ; : = and spaces',
    input: 'password: "a;b:c=d e"',
    expect: { password: ['a;b:c=d e'] },
  },
  {
    name: 'escaped quote in double quotes',
    input: 'password="a\\"b"',
    expect: { password: ['a"b'] },
  },
  {
    name: 'single quote keeps backslash',
    input: "password='a\\b'",
    expect: { password: ['a\\b'] },
  },
  {
    name: 'unterminated quote takes the rest',
    input: 'password="abc',
    expect: { password: ['abc'] },
  },
  {
    name: 'password with ; : = space # and no later label',
    input: 'password: a;b:c=d e #x',
    expect: { password: ['a;b:c=d e #x'] },
  },
  {
    name: 'inline pair split by ;',
    input: 'password=x; user=y',
    expect: { password: ['x'], username: ['y'] },
  },
  {
    name: '.env block with export and comments',
    input: '# creds\nexport PGUSER=alice\n// note\nexport PGPASSWORD="p w"\n',
    expect: { username: ['alice'], password: ['p w'] },
  },
  {
    name: 'libpq env names',
    input: 'PGHOST=db.example.com\nPGPORT=5433\nPGUSER=u\nPGPASSWORD=p\nPGDATABASE=app',
    expect: {
      host: ['db.example.com'],
      port: ['5433'],
      username: ['u'],
      password: ['p'],
      database: ['app'],
    },
  },
  {
    name: 'DB_ prefix',
    input: 'DB_USER=u\nDB_PASSWORD=p',
    expect: { username: ['u'], password: ['p'] },
  },
  { name: 'MYSQL_PWD', kind: 'mysql', input: 'MYSQL_PWD=s3cret', expect: { password: ['s3cret'] } },
  {
    name: 'DSN key=value;',
    input: 'Server=h;Port=5433;User Id=u;Password=p;Database=d',
    expect: { host: ['h'], port: ['5433'], username: ['u'], password: ['p'], database: ['d'] },
  },
  {
    name: 'DSN password holding ; with no later label',
    input: 'Password=a;b;Database=d',
    expect: { password: ['a;b'], database: ['d'] },
  },
  {
    name: 'Data Source / Initial Catalog',
    input: 'Data Source=h;Initial Catalog=d',
    expect: { host: ['h'], database: ['d'] },
  },
  {
    name: 'psql flags with positional db',
    input: 'psql -h db -p 5433 -U alice mydb',
    expect: { host: ['db'], port: ['5433'], username: ['alice'], database: ['mydb'] },
  },
  {
    name: 'env password before psql',
    input: "PGPASSWORD='x y' psql -h db -U alice",
    expect: { password: ['x y'], host: ['db'], username: ['alice'] },
  },
  {
    name: 'mysql attached -p and port',
    kind: 'mysql',
    input: 'mysql -u u -pSecret -h h -P 3307 db',
    expect: {
      username: ['u'],
      password: ['Secret'],
      host: ['h'],
      port: ['3307'],
      database: ['db'],
    },
  },
  {
    name: 'mysql detached -p prompts',
    kind: 'mysql',
    input: 'mysql -u u -p',
    expect: { username: ['u'] },
  },
  {
    name: 'redis-cli',
    kind: 'redis',
    input: 'redis-cli -h r -p 6380 -a "pw 1" -n 2',
    expect: { host: ['r'], port: ['6380'], password: ['pw 1'], database: ['2'] },
  },
  {
    name: 'mongosh auth db',
    kind: 'mongodb',
    input: 'mongosh --host m --port 27018 -u u -p p --authenticationDatabase admin',
    expect: {
      host: ['m'],
      port: ['27018'],
      username: ['u'],
      password: ['p'],
      authSource: ['admin'],
    },
  },
  {
    name: 'kcat sasl properties',
    kind: 'kafka',
    input: 'kcat -b broker:9093 -X sasl.username=u -X sasl.password=p',
    expect: { host: ['broker'], port: ['9093'], username: ['u'], password: ['p'] },
  },
  {
    name: 'JAAS line',
    kind: 'kafka',
    input:
      'sasl.jaas.config=org.apache.kafka.common.security.plain.PlainLoginModule required username="u" password="p";',
    expect: { username: ['u'], password: ['p'] },
  },
  {
    name: 'unknown client dash line, -p numeric is port',
    input: '-h h -p5432',
    expect: { host: ['h'], port: ['5432'] },
  },
  {
    name: 'unknown client dash line, -p text is password',
    input: '-u u -psecret',
    expect: { username: ['u'], password: ['secret'] },
  },
  { name: 'host:port split', input: 'host: h:5433', expect: { host: ['h'], port: ['5433'] } },
  {
    name: 'bracketed IPv6 with port',
    input: 'host=[::1]:5433',
    expect: { host: ['::1'], port: ['5433'] },
  },
  { name: 'port out of range dropped', input: 'host=h\nport=70000', expect: { host: ['h'] } },
  { name: 'redis non-numeric database dropped', kind: 'redis', input: 'db=cache', expect: {} },
  {
    name: 'kafka brokers: first only',
    kind: 'kafka',
    input: 'brokers=a:9092,b:9092',
    expect: { host: ['a'], port: ['9092'] },
  },
  {
    name: 'kafka has no database',
    kind: 'kafka',
    input: 'user=u\ndatabase=d',
    expect: { username: ['u'] },
  },
  {
    name: 'postgres service maps to database',
    input: 'service=svc',
    expect: { database: ['svc'] },
  },
  { name: 'schema is not a database', input: 'schema=public', expect: {} },
  {
    name: 'authSource only for mongodb',
    input: 'authSource=admin\nuser=u',
    expect: { username: ['u'] },
  },
  { name: 'URI in host is rejected', input: 'host=postgres://u:p@h/db', expect: {} },
  {
    name: 'AWS env: region and profile kept, keys never valued',
    kind: 's3',
    input:
      'AWS_REGION=eu-west-1\nAWS_PROFILE=dev\nAWS_ACCESS_KEY_ID=AKIAXXXX\nAWS_SECRET_ACCESS_KEY=zzz\nAWS_SESSION_TOKEN=ttt',
    expect: { region: ['eu-west-1'], profile: ['dev'] },
  },
  { name: 'sqlite takes nothing', kind: 'sqlite', input: 'user=u\npassword=p', expect: {} },
  {
    name: 'unlabelled two lines',
    input: 'alice\nhunter2',
    expect: { username: ['alice'], password: ['hunter2'] },
  },
  {
    name: 'unlabelled two tokens',
    input: 'alice hunter2',
    expect: { username: ['alice'], password: ['hunter2'] },
  },
  { name: 'unlabelled single token', input: 'hunter2', expect: { password: ['hunter2'] } },
  {
    name: 'prose yields nothing',
    input: 'please send me the database credentials today',
    expect: {},
  },
  {
    name: 'unlabelled fallback skipped when a host was found',
    input: 'host: h',
    expect: { host: ['h'] },
  },
  { name: 'duplicates collapse', input: 'user=u\nUSERNAME=u', expect: { username: ['u'] } },
  {
    name: 'conflicting candidates keep document order',
    input: 'user=a\nlogin=b',
    expect: { username: ['a', 'b'] },
  },
  {
    name: 'CRLF and BOM',
    input: '﻿user=a\r\npass=b\r\n',
    expect: { username: ['a'], password: ['b'] },
  },
];

describe('parseCredentialPaste', () => {
  for (const row of ROWS) {
    test(row.name, () => {
      expect(values(row.input, row.kind ?? 'postgres')).toEqual(row.expect);
    });
  }

  test('8 KiB cap parses nothing', () => {
    const r = parseCredentialPaste(`password=${'a'.repeat(MAX_PASTE_BYTES)}`, 'postgres');
    expect(r.tooLong).toBe(true);
    expect(r.fields).toEqual({});
  });

  test('notes never carry a value; unrecognised lines are counted', () => {
    const r = parseCredentialPaste('AWS_SECRET_ACCESS_KEY=topsecret\nwhatever line', 's3');
    expect(JSON.stringify(r)).not.toContain('topsecret');
    expect(r.unrecognised).toBe(1);
  });

  test('shell expansion is never applied', () => {
    expect(values('mysql -u u -p"$HOME"x', 'mysql').password).toEqual(['$HOMEx']);
  });

  test('labels and guessed flags', () => {
    const r = parseCredentialPaste('service=svc\nPGPASSWORD=p', 'postgres');
    expect(r.fields.database?.[0]).toMatchObject({ guessed: true, label: 'service' });
    expect(r.fields.password?.[0]).toMatchObject({ guessed: false, label: 'PGPASSWORD' });
  });
});

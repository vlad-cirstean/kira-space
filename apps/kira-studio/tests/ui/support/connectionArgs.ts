import { newConnectionArgs } from './connect';

// The `connections.create` args the new-connection dialog sends for each engine. The colour and
// connection fields live in the file, so the call carries none.
export function mongoCreateArgs(name: string) {
  return newConnectionArgs(name, 'mongodb');
}

export function redisCreateArgs(name: string) {
  return newConnectionArgs(name, 'redis');
}

export function postgresCreateArgs(name: string, _color?: string) {
  return newConnectionArgs(name, 'postgres');
}

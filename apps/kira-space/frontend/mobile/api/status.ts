import {
  type PhoneComputer,
  type PhoneRun,
  phoneComputerSchema,
  phoneRunsSchema,
} from '@shared/domain/mobile';
import { getJson } from './http';

export async function getPhoneRuns(): Promise<PhoneRun[]> {
  return phoneRunsSchema.parse(await getJson('/api/runs')).runs;
}

export async function getPhoneComputer(): Promise<PhoneComputer> {
  return phoneComputerSchema.parse(await getJson('/api/computer'));
}

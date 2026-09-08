import type { Profile } from './api';
import { firstAvailableNumber } from './available-number.ts';

export function profileSaveMode(
  originalID: string | undefined,
  enteredID: string,
): 'create' | 'update' | 'copy' {
  if (!originalID) return 'create';
  return originalID === enteredID.trim() ? 'update' : 'copy';
}

export function nextTestUserNumber(profiles: Pick<Profile, 'userId'>[]) {
  return firstAvailableNumber(
    profiles.map((profile) => profile.userId),
    /^user-(\d+)$/,
  );
}

export function testUserID(sequence: number) {
  return `user-${String(sequence).padStart(4, '0')}`;
}

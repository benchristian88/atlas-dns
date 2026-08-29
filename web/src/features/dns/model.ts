import { validateNetwork } from "../../components/StructuredInputs";

const SECOND = 1;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

export const DNS_DURATION_UNITS = [
  { label: "seconds", multiplier: SECOND },
  { label: "minutes", multiplier: MINUTE },
  { label: "hours", multiplier: HOUR },
  { label: "days", multiplier: DAY },
] as const;

export const TTL_PRESETS = [
  { label: "No override", value: 0 },
  { label: "1 minute", value: MINUTE },
  { label: "5 minutes", value: 5 * MINUTE },
  { label: "1 hour", value: HOUR },
  { label: "1 day", value: DAY },
] as const;

export const TIMEOUT_PRESETS = [
  { label: "Node default", value: 0 },
  { label: "5 seconds", value: 5 },
  { label: "10 seconds", value: 10 },
  { label: "30 seconds", value: 30 },
] as const;

export type CacheSizeUnit = "bytes" | "KiB" | "MiB";

const CACHE_SIZE_MULTIPLIERS: Record<CacheSizeUnit, number> = {
  bytes: 1,
  KiB: 1024,
  MiB: 1024 * 1024,
};

export function cacheSizeToBytes(value: number, unit: CacheSizeUnit): number {
  return value * CACHE_SIZE_MULTIPLIERS[unit];
}

export function cacheSizeForDisplay(bytes: number): {
  value: number;
  unit: CacheSizeUnit;
} {
  if (bytes !== 0 && bytes % CACHE_SIZE_MULTIPLIERS.MiB === 0)
    return { value: bytes / CACHE_SIZE_MULTIPLIERS.MiB, unit: "MiB" };
  if (bytes !== 0 && bytes % CACHE_SIZE_MULTIPLIERS.KiB === 0)
    return { value: bytes / CACHE_SIZE_MULTIPLIERS.KiB, unit: "KiB" };
  return { value: bytes, unit: "bytes" };
}

export function validCacheSize(bytes: number, enabled: boolean): boolean {
  return Number.isSafeInteger(bytes) && bytes >= 0 && (!enabled || bytes > 0);
}

export function validWholeSeconds(value: number): boolean {
  return Number.isSafeInteger(value) && value >= 0;
}

export function validateIpFamily(
  value: string,
  family: 4 | 6,
): string | undefined {
  if (value.trim() === "") return undefined;
  const networkError = validateNetwork(value, true, false);
  if (networkError !== undefined)
    return family === 4
      ? "Enter a valid IPv4 address."
      : "Enter a valid IPv6 address.";
  const isIPv6 = value.trim().includes(":");
  if ((family === 6) !== isIPv6)
    return family === 4 ? "Enter an IPv4 address." : "Enter an IPv6 address.";
  return undefined;
}

export function validateIp(value: string): string | undefined {
  return value.trim() === "" ? undefined : validateNetwork(value, true, false);
}

import {
  validateDomain,
  validateNetwork,
} from "../../components/StructuredInputs";
import type { Rewrite } from "../../lib/types";

export type RewriteType =
  | "A"
  | "AAAA"
  | "CNAME"
  | "CNAME exception"
  | "A passthrough"
  | "AAAA passthrough"
  | "Unknown";

export type RewriteChangeState = "added" | "modified" | "unchanged";

export interface RewriteValidation {
  domain?: string;
  answer?: string;
  duplicate?: string;
}

export function validateRewrite(
  rewrite: Rewrite,
  rewrites: readonly Rewrite[],
  editingIndex?: number,
): RewriteValidation {
  const domain = rewrite.domain.trim();
  const answer = rewrite.answer.trim();
  const domainError = validateRewriteDomain(domain);
  const answerError = validateRewriteAnswer(answer);
  const duplicate = rewrites.some(
    (item, index) =>
      index !== editingIndex &&
      item.domain.trim().toLocaleLowerCase() === domain.toLocaleLowerCase() &&
      item.answer.trim().toLocaleLowerCase() === answer.toLocaleLowerCase(),
  )
    ? "This domain and answer pair already exists in the draft."
    : undefined;
  return { domain: domainError, answer: answerError, duplicate };
}

export function hasRewriteValidationErrors(validation: RewriteValidation) {
  return Object.values(validation).some((value) => value !== undefined);
}

function validateRewriteDomain(value: string): string | undefined {
  const candidate = value.trim();
  if (candidate === "") return "Enter a domain or leading *. wildcard.";
  if (candidate.endsWith("."))
    return "Enter the domain without a trailing dot.";
  if (validateDomain(candidate, true) !== undefined)
    return "Enter a valid hostname or a wildcard beginning with *.";
  return undefined;
}

function validateRewriteAnswer(value: string): string | undefined {
  const candidate = value.trim();
  if (candidate === "") return "Enter an IP address, hostname, A, or AAAA.";
  if (candidate === "A" || candidate === "AAAA") return undefined;
  if (validateNetwork(candidate, true, false) === undefined) return undefined;
  if (/^[0-9.]+$/.test(candidate)) return "Enter a valid IPv4 address.";
  if (candidate.endsWith("."))
    return "Enter the CNAME hostname without a trailing dot.";
  if (candidate.includes("*") || validateDomain(candidate) !== undefined)
    return "Enter a valid IPv4 address, IPv6 address, CNAME hostname, A, or AAAA.";
  return undefined;
}

export function inferRewriteType(
  rewrite: Pick<Rewrite, "domain" | "answer">,
): RewriteType {
  const answer = rewrite.answer.trim();
  if (answer === "A") return "A passthrough";
  if (answer === "AAAA") return "AAAA passthrough";
  if (validateNetwork(answer, true, false) === undefined)
    return answer.includes(":") ? "AAAA" : "A";
  if (validateRewriteAnswer(answer) !== undefined) return "Unknown";
  return rewrite.domain.trim().toLocaleLowerCase() ===
    answer.toLocaleLowerCase()
    ? "CNAME exception"
    : "CNAME";
}

export function rewriteMatchesSearch(rewrite: Rewrite, search: string) {
  const query = search.trim().toLocaleLowerCase();
  if (query === "") return true;
  return [rewrite.domain, rewrite.answer].some((value) =>
    value.toLocaleLowerCase().includes(query),
  );
}

export function rewriteChangeState(
  rewrite: Rewrite,
  index: number,
  savedRewrites: readonly Rewrite[],
  currentRewrites: readonly Rewrite[] = [rewrite],
): RewriteChangeState {
  const savedPair = savedRewrites.find((saved) => samePair(saved, rewrite));
  if (savedPair !== undefined)
    return JSON.stringify(savedPair) === JSON.stringify(rewrite)
      ? "unchanged"
      : "modified";
  const previousAtIndex = savedRewrites[index];
  if (
    previousAtIndex !== undefined &&
    !currentRewrites.some((current) => samePair(current, previousAtIndex))
  )
    return "modified";
  return "added";
}

export function cleanRewriteForDraft(rewrite: Rewrite): Rewrite {
  return {
    ...rewrite,
    domain: rewrite.domain.trim(),
    answer: rewrite.answer.trim(),
  };
}

function samePair(left: Rewrite, right: Rewrite) {
  return (
    left.domain.trim().toLocaleLowerCase() ===
      right.domain.trim().toLocaleLowerCase() &&
    left.answer.trim().toLocaleLowerCase() ===
      right.answer.trim().toLocaleLowerCase()
  );
}

import {
  createContext,
  type ReactNode,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";

export type ThemePreference = "system" | "light" | "dark";
export type ResolvedTheme = Exclude<ThemePreference, "system">;

export const THEME_STORAGE_KEY = "atlas-dns.theme";
const THEME_MEDIA_QUERY = "(prefers-color-scheme: dark)";
export const THEME_COLORS: Record<ResolvedTheme, string> = {
  light: "#ffffff",
  dark: "#1b222a",
};

interface ThemeContextValue {
  preference: ThemePreference;
  resolvedTheme: ResolvedTheme;
  setPreference: (preference: ThemePreference) => void;
}

const ThemeContext = createContext<ThemeContextValue | undefined>(undefined);

function isThemePreference(value: string | null): value is ThemePreference {
  return value === "system" || value === "light" || value === "dark";
}

function storedPreference(): ThemePreference {
  try {
    const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
    return isThemePreference(stored) ? stored : "system";
  } catch {
    return "system";
  }
}

function systemTheme(query: MediaQueryList): ResolvedTheme {
  return query.matches ? "dark" : "light";
}

function applyDocumentTheme(theme: ResolvedTheme, preference: ThemePreference) {
  document.documentElement.dataset.theme = theme;
  document.documentElement.dataset.themePreference = preference;
  document.documentElement.style.colorScheme = theme;

  const themeColor = document.querySelector<HTMLMetaElement>(
    'meta[name="theme-color"]',
  );
  themeColor?.setAttribute("content", THEME_COLORS[theme]);
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] = useState<ThemePreference>(() =>
    storedPreference(),
  );
  const [resolvedTheme, setResolvedTheme] = useState<ResolvedTheme>(() => {
    if (preference !== "system") return preference;
    return systemTheme(window.matchMedia(THEME_MEDIA_QUERY));
  });

  useEffect(() => {
    const query = window.matchMedia(THEME_MEDIA_QUERY);

    const synchronize = () => {
      const resolved =
        preference === "system" ? systemTheme(query) : preference;
      setResolvedTheme(resolved);
      applyDocumentTheme(resolved, preference);
    };

    synchronize();
    if (preference !== "system") return;
    query.addEventListener("change", synchronize);
    return () => query.removeEventListener("change", synchronize);
  }, [preference]);

  const value = useMemo<ThemeContextValue>(
    () => ({
      preference,
      resolvedTheme,
      setPreference: (next) => {
        setPreferenceState(next);
        try {
          window.localStorage.setItem(THEME_STORAGE_KEY, next);
        } catch {
          // A blocked or full storage area must not make theme selection fail.
        }
      },
    }),
    [preference, resolvedTheme],
  );

  return (
    <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
  );
}

export function useTheme(): ThemeContextValue {
  const value = useContext(ThemeContext);
  if (value === undefined) {
    throw new Error("useTheme must be used inside ThemeProvider");
  }
  return value;
}

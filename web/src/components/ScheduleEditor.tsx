import { useId } from "react";
import type { Schedule } from "../lib/types";
import { Field } from "./Settings";

const scheduleDays = [
  ["sun", "Sunday"],
  ["mon", "Monday"],
  ["tue", "Tuesday"],
  ["wed", "Wednesday"],
  ["thu", "Thursday"],
  ["fri", "Friday"],
  ["sat", "Saturday"],
] as const;

function formatTimeOfDay(milliseconds: number) {
  const totalMinutes = Math.round(milliseconds / 60_000);
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return `${String(hours).padStart(2, "0")}:${String(minutes).padStart(2, "0")}`;
}

function parseTimeOfDay(value: string) {
  const [hours = 0, minutes = 0] = value.split(":").map(Number);
  return (hours * 60 + minutes) * 60_000;
}

export function ScheduleEditor({
  value,
  onChange,
  label,
  errors = [],
}: {
  value: Schedule;
  onChange: (value: Schedule) => void;
  label: string;
  errors?: string[];
}) {
  const timeZoneID = useId();
  const schedule = value ?? { timeZone: "Local", days: {} };
  const updateDay = (day: string, enabled: boolean) => {
    const days = { ...schedule.days };
    if (enabled) days[day] = days[day] ?? { start: 0, end: 86_400_000 };
    else delete days[day];
    onChange({ ...schedule, days });
  };
  return (
    <fieldset className="schedule-editor">
      <legend className="visually-hidden">{label}</legend>
      <h3 className="schedule-editor__title">{label}</h3>
      <Field
        label="Time zone"
        htmlFor={timeZoneID}
        help="Use Local or an IANA time zone such as Pacific/Auckland."
        error={errors.find((error) => error.includes("timeZone"))}
      >
        <input
          id={timeZoneID}
          value={schedule.timeZone || "Local"}
          aria-invalid={errors.some((error) => error.includes("timeZone"))}
          onChange={(event) =>
            onChange({ ...schedule, timeZone: event.target.value })
          }
        />
      </Field>
      <div className="schedule-grid">
        {scheduleDays.map(([day, dayLabel]) => {
          const range = schedule.days[day];
          const dayError = errors.find((error) =>
            error.includes(`days.${day}`),
          );
          return (
            <div className="schedule-row" key={day}>
              <label className="checkbox">
                <input
                  type="checkbox"
                  checked={Boolean(range)}
                  onChange={(event) => updateDay(day, event.target.checked)}
                />
                <span>{dayLabel}</span>
              </label>
              <input
                aria-label={`${dayLabel} start`}
                aria-invalid={Boolean(dayError)}
                type="time"
                step={60}
                disabled={!range}
                value={range ? formatTimeOfDay(range.start) : "00:00"}
                onChange={(event) =>
                  onChange({
                    ...schedule,
                    days: {
                      ...schedule.days,
                      [day]: {
                        ...(range ?? { end: 86_400_000 }),
                        start: parseTimeOfDay(event.target.value),
                      },
                    },
                  })
                }
              />
              <input
                aria-label={`${dayLabel} end`}
                aria-invalid={Boolean(dayError)}
                type="time"
                step={60}
                disabled={!range}
                value={
                  range?.end === 86_400_000
                    ? "23:59"
                    : formatTimeOfDay(range?.end ?? 86_400_000)
                }
                onChange={(event) =>
                  onChange({
                    ...schedule,
                    days: {
                      ...schedule.days,
                      [day]: {
                        ...(range ?? { start: 0 }),
                        end: parseTimeOfDay(event.target.value),
                      },
                    },
                  })
                }
              />
              {dayError !== undefined && (
                <span className="field__error schedule-row__error" role="alert">
                  {dayError}
                </span>
              )}
            </div>
          );
        })}
      </div>
      <p className="muted">
        A selected day is the period when blocked-service filtering is inactive.
      </p>
    </fieldset>
  );
}

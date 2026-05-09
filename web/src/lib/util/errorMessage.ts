// Tiny helper for the catch-block pattern:
//
//   } catch (err) {
//     toast.error('failed: ' + (err instanceof Error ? err.message : String(err)));
//   }
//
// 50+ files had the same instance-check spelled out. Centralising it
// here gives every catch block a one-liner that reads as intent
// ("the error message, however it's typed") rather than ceremony.
//
// Returns a non-empty string for every input — `String(null)` and
// `String(undefined)` both produce stable strings, so a thrown null
// doesn't render an empty toast.

export function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  return String(err);
}

/**
 * Formats a time window duration with abbreviated units (h/d)
 * @param duration Time window duration in nanoseconds
 * @returns Formatted string with h/d units - 'h' if less than 24 hours, 'd' if 24 hours or more
 */
export function formatTimeWindow(duration: number): string {
  // Convert nanoseconds to hours
  const hours = Math.floor(duration / (60 * 60 * 1000000000));
  
  // Use 'h' for durations less than 24 hours
  if (hours < 24) {
    return `${hours}h`;
  }
  
  // Use 'd' for durations of 24 hours or more
  const days = Math.floor(hours / 24);
  return `${days}d`;
}

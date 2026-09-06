<script lang="ts">
  let {
    samples = [],
    available = true,
    compact = false,
  }: { samples?: { down: number; up: number }[]; available?: boolean; compact?: boolean } = $props();
  const maximum = $derived(Math.max(1000, ...samples.flatMap((s) => [s.down, s.up])) * 1.15);
  function points(direction: 'down' | 'up') {
    return samples
      .map(
        (s, i) => `${600 - (samples.length - 1 - i) * (600 / 119)},${116 - (s[direction] / maximum) * 100}`,
      )
      .join(' ');
  }
</script>

<div class:compact class="traffic-chart" aria-label="Download and upload over the last minute">
  <div class="chart-scale">
    <span>{available ? (maximum / 1_000_000).toFixed(1) + ' MB/s' : '—'}</span><span>0</span>
  </div>
  <svg viewBox="0 0 600 124" preserveAspectRatio="none" role="img" aria-label="Traffic history">
    <path d="M0 16H600 M0 66H600 M0 116H600" class="chart-grid" />
    {#if available && samples.length > 1}
      <polyline points={points('down')} class="chart-download" />
      <polyline points={points('up')} class="chart-upload" />
    {/if}
  </svg>
  {#if !available}<span class="chart-unavailable">Traffic history appears when the service is connected</span
    >{/if}
  <div class="chart-times"><span>60 seconds ago</span><span>Now</span></div>
</div>

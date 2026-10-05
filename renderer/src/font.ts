// This profile always uses the bundled font at Regular (400).
export const fontFamily = 'Glypha Noto Sans JP';
export const fontWeight = '400' as const;

export async function loadDisplayFont(): Promise<void> {
  let timeout: ReturnType<typeof setTimeout> | undefined;
  try {
    const faces = await Promise.race([
      document.fonts.load(`${fontWeight} 32px "${fontFamily}"`),
      new Promise<never>((_, reject) => {
        timeout = setTimeout(() => reject(new Error('Display font loading timed out.')), 15000);
      }),
    ]);
    if (faces.length === 0 || faces.some((face) => face.status !== 'loaded')) {
      throw new Error('The bundled display font is unavailable.');
    }
  } finally {
    clearTimeout(timeout);
  }
}

// The API's pattern for an Indonesian mobile number (openapi.yaml), checked before sending so
// the form can say what is wrong. Spaces and dashes are allowed while typing.
const mobilePattern = /^(\+62|62|0)8[0-9]{8,11}$/;

export function compactPhone(typed: string): string {
  return typed.replace(/[\s-]/g, "");
}

export function isIndonesianMobile(typed: string): boolean {
  return mobilePattern.test(compactPhone(typed));
}

export const phoneHint = "Tulis nomor WhatsApp yang dimulai 08, 628, atau +628.";

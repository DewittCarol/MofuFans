export type PetReaction = { value: string; name?: string; url?: string };
export type PetReactionProfile = { face: string; action: string; pngExpression: number };

// Uploaded image names can opt into the same behavior; unknown images still pop
// above the pet and receive a friendly default response.
export function petReactionProfile(reaction: PetReaction): PetReactionProfile {
  const text = `${reaction.value} ${reaction.name ?? ''}`.toLowerCase();
  if (/[🐵🙈🙉🙊😂🤣😏👅💩]|猴|嘲笑|调皮|坏笑|笑哭|monkey|laugh|lol/u.test(text)) return { face: 'teasing', action: 'giggle', pngExpression: 2 };
  if (/[😭😢🥲😿🥺😓]|哭|委屈|难过|伤心|失落|cry|sad|tear/u.test(text)) return { face: 'sad', action: 'comfort', pngExpression: 1 };
  if (/[❓❔🤔⁉❗😨😬]|疑问|疑惑|问号|question|confus/u.test(text)) return { face: 'curious', action: 'tilt', pngExpression: 0 };
  if (/[😡😠💢🤬👎]|生气|愤怒|angry|rage/u.test(text)) return { face: 'angry', action: 'shake', pngExpression: 1 };
  if (/[👍👏🎉💯🥳]|点赞|赞|厉害|好棒|thumb|like|great|good/u.test(text)) return { face: 'happy', action: 'nod', pngExpression: 2 };
  if (/[❤♥💕💖🥰😍🫂🌹]|爱|喜欢|贴贴|love|heart/u.test(text)) return { face: 'happy', action: 'wave', pngExpression: 2 };
  return { face: 'happy', action: 'wave', pngExpression: 2 };
}

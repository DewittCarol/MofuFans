'use client';
import {BilibiliPerson} from '@/components/bilibili-person';
import {useBilibiliProfiles} from '@/lib/use-bilibili-profiles';
import type {VoyageRecipient} from '@/lib/voyage-recipients';
export function VoyagePerson({person}:{person:VoyageRecipient}){
 const profiles=useBilibiliProfiles([person.uid]);
 return <BilibiliPerson uid={person.uid} name={person.name} avatar={person.avatar} profile={profiles[person.uid]}/>;
}

import type { User } from "@/types/models";

const ago = (min: number) => new Date(Date.now() - min * 60000).toISOString();

/** People you can chat with. (The signed-in user is created by auth.) */
export const PEOPLE: User[] = [
  { id: "u_aisha", username: "aisha", displayName: "Aisha Rahman", bio: "Design lead ☕", presence: "online", createdAt: ago(100000) },
  { id: "u_rafi", username: "rafi_k", displayName: "Rafi Khan", bio: "Shipping things.", presence: "online", createdAt: ago(90000) },
  { id: "u_nabila", username: "nabila", displayName: "Nabila Ahmed", presence: "away", lastSeenAt: ago(12), createdAt: ago(80000) },
  { id: "u_tanvir", username: "tanvir", displayName: "Tanvir Hasan", presence: "offline", lastSeenAt: ago(190), createdAt: ago(70000) },
  { id: "u_mim", username: "mim", displayName: "Mim Chowdhury", bio: "Coffee, code, repeat.", presence: "online", createdAt: ago(60000) },
  { id: "u_sakib", username: "sakib99", displayName: "Sakib Islam", presence: "offline", lastSeenAt: ago(1500), createdAt: ago(50000) },
  { id: "u_farhan", username: "farhan", displayName: "Farhan Ali", presence: "offline", lastSeenAt: ago(4000), createdAt: ago(45000) },
  { id: "u_lamia", username: "lamia", displayName: "Lamia Sultana", presence: "online", createdAt: ago(40000) },
  { id: "u_imran", username: "imran", displayName: "Imran Hossain", presence: "offline", lastSeenAt: ago(800), createdAt: ago(30000) },
  { id: "u_sadia", username: "sadia", displayName: "Sadia Karim", presence: "online", createdAt: ago(20000) },
  { id: "u_zubair", username: "zubair", displayName: "Zubair Alam", presence: "offline", lastSeenAt: ago(9000), createdAt: ago(10000) },
  { id: "u_ruma", username: "ruma", displayName: "Ruma Akter", presence: "offline", lastSeenAt: ago(300), createdAt: ago(9000) },
];
